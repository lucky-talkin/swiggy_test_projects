package main

import (
    "context"
    "encoding/json"
    "errors"
    "log"
    "net/http"
    "os"
    "strings"
    "time"

    "github.com/jackc/pgx/v5"
    "github.com/jackc/pgx/v5/pgxpool"
)

type app struct { db *pgxpool.Pool; orderAPI,token string }
type assignmentRequest struct { OrderID string `json:"order_id"`; RestaurantID string `json:"restaurant_id"` }
type assignmentResponse struct { OrderID string `json:"order_id"`; DriverID string `json:"driver_id"`; Status string `json:"status"` }

func main() {
    ctx:=context.Background()
    db,err:=pgxpool.New(ctx,env("DATABASE_URL","postgres://food:food@localhost:5432/food_delivery?sslmode=disable"))
    if err!=nil { log.Fatal(err) }; defer db.Close()
    if err=db.Ping(ctx); err!=nil { log.Fatal(err) }
    if err=migrate(db); err!=nil { log.Fatal(err) }
    for _,id:=range []string{"driver-demo-001","driver-demo-002","driver-demo-003"} {
        if _,err=db.Exec(ctx,"INSERT INTO drivers(driver_id,status) VALUES($1,'AVAILABLE') ON CONFLICT(driver_id) DO NOTHING",id); err!=nil { log.Fatal(err) }
    }
    a:=&app{db:db,orderAPI:env("ORDER_API_URL","http://localhost:8086"),token:env("INTERNAL_SERVICE_TOKEN","dev-internal-token")}
    go a.dispatchOutbox(ctx)

    mux:=http.NewServeMux()
    mux.HandleFunc("GET /healthz",a.health)
    mux.HandleFunc("POST /v1/drivers/{driver_id}/availability",a.setAvailability)
    mux.HandleFunc("POST /v1/assignments",a.assign)
    mux.HandleFunc("POST /v1/assignments/{order_id}/picked-up",func(w http.ResponseWriter,r *http.Request){a.transition(w,r,"PICKED_UP","driver_picked_up")})
    mux.HandleFunc("POST /v1/assignments/{order_id}/delivered",func(w http.ResponseWriter,r *http.Request){a.transition(w,r,"DELIVERED","delivery_completed")})
    mux.HandleFunc("POST /v1/assignments/{order_id}/release",a.release)
    a.registerPortalRoutes(mux)
    port:=env("PORT","8085")
    log.Printf("driver-ops listening on :%s",port)
    log.Fatal((&http.Server{Addr:":"+port,Handler:mux,ReadHeaderTimeout:5*time.Second}).ListenAndServe())
}

func (a *app) health(w http.ResponseWriter,r *http.Request) {
    ctx,cancel:=context.WithTimeout(r.Context(),2*time.Second); defer cancel()
    if err:=a.db.Ping(ctx); err!=nil { fail(w,503,"DATABASE_UNAVAILABLE",err.Error()); return }
    respond(w,200,map[string]string{"service":"driver-ops","status":"ok"})
}

func (a *app) setAvailability(w http.ResponseWriter,r *http.Request) {
    if !a.authorized(r) { fail(w,401,"UNAUTHORIZED","invalid service token"); return }
    var req struct { Available bool `json:"available"` }
    if err:=json.NewDecoder(r.Body).Decode(&req); err!=nil { fail(w,400,"INVALID_JSON",err.Error()); return }
    target:="OFFLINE"; if req.Available { target="AVAILABLE" }
    id:=r.PathValue("driver_id")
    tag,err:=a.db.Exec(r.Context(),`
        INSERT INTO drivers(driver_id,status) VALUES($1,$2)
        ON CONFLICT(driver_id) DO UPDATE SET status=EXCLUDED.status,updated_at=NOW()
        WHERE drivers.status IN ('AVAILABLE','OFFLINE')`,id,target)
    if err!=nil { fail(w,500,"DATABASE_ERROR",err.Error()); return }
    if tag.RowsAffected()==0 { fail(w,409,"DRIVER_BUSY","cannot change availability while assigned or delivering"); return }
    respond(w,200,map[string]string{"driver_id":id,"status":target})
}

func (a *app) assign(w http.ResponseWriter,r *http.Request) {
    if !a.authorized(r) { fail(w,401,"UNAUTHORIZED","invalid service token"); return }
    var req assignmentRequest
    if err:=json.NewDecoder(http.MaxBytesReader(w,r.Body,1<<20)).Decode(&req); err!=nil { fail(w,400,"INVALID_JSON",err.Error()); return }
    if req.OrderID=="" || req.RestaurantID=="" { fail(w,400,"VALIDATION_ERROR","order_id and restaurant_id are required"); return }
    tx,err:=a.db.Begin(r.Context()); if err!=nil { fail(w,500,"DATABASE_ERROR",err.Error()); return }
    defer tx.Rollback(r.Context())

    var existingDriver,existingStatus string
    err=tx.QueryRow(r.Context(),"SELECT driver_id,status FROM driver_assignments WHERE order_id=$1 FOR UPDATE",req.OrderID).Scan(&existingDriver,&existingStatus)
    if err==nil && (existingStatus=="ASSIGNED" || existingStatus=="PICKED_UP") {
        _=tx.Commit(r.Context()); respond(w,200,assignmentResponse{req.OrderID,existingDriver,existingStatus}); return
    }
    if err==nil && existingStatus!="RELEASED" { fail(w,409,"ASSIGNMENT_TERMINAL","assignment is "+existingStatus); return }
    if err!=nil && !errors.Is(err,pgx.ErrNoRows) { fail(w,500,"DATABASE_ERROR",err.Error()); return }

    var driverID string
    err=tx.QueryRow(r.Context(),`SELECT driver_id FROM drivers WHERE status='AVAILABLE' ORDER BY driver_id LIMIT 1 FOR UPDATE SKIP LOCKED`).Scan(&driverID)
    if errors.Is(err,pgx.ErrNoRows) { fail(w,503,"NO_DRIVER_AVAILABLE","no available drivers; Temporal will retry"); return }
    if err!=nil { fail(w,500,"DATABASE_ERROR",err.Error()); return }

    _,err=tx.Exec(r.Context(),"UPDATE drivers SET status='ASSIGNED',updated_at=NOW() WHERE driver_id=$1",driverID)
    if err!=nil { fail(w,500,"DATABASE_ERROR",err.Error()); return }
    if existingStatus=="RELEASED" {
        _,err=tx.Exec(r.Context(),`UPDATE driver_assignments SET driver_id=$2,restaurant_id=$3,status='ASSIGNED',updated_at=NOW() WHERE order_id=$1`,req.OrderID,driverID,req.RestaurantID)
    } else {
        _,err=tx.Exec(r.Context(),`INSERT INTO driver_assignments(order_id,driver_id,restaurant_id,status) VALUES($1,$2,$3,'ASSIGNED')`,req.OrderID,driverID,req.RestaurantID)
    }
    if err!=nil { fail(w,500,"DATABASE_ERROR",err.Error()); return }
    if err=tx.Commit(r.Context()); err!=nil { fail(w,500,"DATABASE_ERROR",err.Error()); return }
    respond(w,201,assignmentResponse{req.OrderID,driverID,"ASSIGNED"})
}

func (a *app) transition(w http.ResponseWriter,r *http.Request,target,signal string) {
    if !a.authorized(r) { fail(w,401,"UNAUTHORIZED","invalid service token"); return }
    id:=r.PathValue("order_id")
    tx,err:=a.db.Begin(r.Context()); if err!=nil { fail(w,500,"DATABASE_ERROR",err.Error()); return }
    defer tx.Rollback(r.Context())
    var driverID,current string
    err=tx.QueryRow(r.Context(),"SELECT driver_id,status FROM driver_assignments WHERE order_id=$1 FOR UPDATE",id).Scan(&driverID,&current)
    if errors.Is(err,pgx.ErrNoRows) { fail(w,404,"ASSIGNMENT_NOT_FOUND","driver assignment does not exist"); return }
    if err!=nil { fail(w,500,"DATABASE_ERROR",err.Error()); return }
    if target=="PICKED_UP" {
        if current!="ASSIGNED" && current!="PICKED_UP" { fail(w,409,"INVALID_ASSIGNMENT_TRANSITION","cannot pick up assignment in "+current); return }
    } else if current!="PICKED_UP" && current!="DELIVERED" {
        fail(w,409,"INVALID_ASSIGNMENT_TRANSITION","cannot deliver assignment in "+current); return
    }

    if current!=target {
        _,err=tx.Exec(r.Context(),"UPDATE driver_assignments SET status=$2,updated_at=NOW() WHERE order_id=$1",id,target)
        if err!=nil { fail(w,500,"DATABASE_ERROR",err.Error()); return }
    }
    if target=="DELIVERED" {
        _,err=tx.Exec(r.Context(),"UPDATE drivers SET status='AVAILABLE',updated_at=NOW() WHERE driver_id=$1",driverID)
        if err!=nil { fail(w,500,"DATABASE_ERROR",err.Error()); return }
    }
    body,_:=json.Marshal(map[string]string{"driver_id":driverID})
    _,err=tx.Exec(r.Context(),`INSERT INTO driver_signal_outbox(order_id,signal_name,payload) VALUES($1,$2,$3)`,id,signal,body)
    if err!=nil { fail(w,500,"OUTBOX_ERROR",err.Error()); return }
    if err=tx.Commit(r.Context()); err!=nil { fail(w,500,"DATABASE_ERROR",err.Error()); return }
    respond(w,202,assignmentResponse{id,driverID,target})
}

func (a *app) release(w http.ResponseWriter,r *http.Request) {
    if !a.authorized(r) { fail(w,401,"UNAUTHORIZED","invalid service token"); return }
    id:=r.PathValue("order_id")
    tx,err:=a.db.Begin(r.Context()); if err!=nil { fail(w,500,"DATABASE_ERROR",err.Error()); return }
    defer tx.Rollback(r.Context())
    var driverID,current string
    err=tx.QueryRow(r.Context(),"SELECT driver_id,status FROM driver_assignments WHERE order_id=$1 FOR UPDATE",id).Scan(&driverID,&current)
    if errors.Is(err,pgx.ErrNoRows) { _=tx.Commit(r.Context()); respond(w,200,map[string]string{"order_id":id,"status":"NOT_ASSIGNED"}); return }
    if err!=nil { fail(w,500,"DATABASE_ERROR",err.Error()); return }

    result:="RELEASED"
    switch current {
    case "ASSIGNED":
        _,err=tx.Exec(r.Context(),"UPDATE driver_assignments SET status='RELEASED',updated_at=NOW() WHERE order_id=$1",id)
        if err==nil { _,err=tx.Exec(r.Context(),"UPDATE drivers SET status='AVAILABLE',updated_at=NOW() WHERE driver_id=$1",driverID) }
    case "PICKED_UP":
        // A driver carrying the order cannot return to the available pool.
        result="DELIVERY_EXCEPTION"
        _,err=tx.Exec(r.Context(),"UPDATE driver_assignments SET status='DELIVERY_EXCEPTION',updated_at=NOW() WHERE order_id=$1",id)
        if err==nil { _,err=tx.Exec(r.Context(),"UPDATE drivers SET status='DELIVERY_EXCEPTION',updated_at=NOW() WHERE driver_id=$1",driverID) }
    case "RELEASED","DELIVERED","DELIVERY_EXCEPTION":
    default:
        err=errors.New("unknown assignment state "+current)
    }
    if err!=nil { fail(w,500,"COMPENSATION_FAILED",err.Error()); return }
    if err=tx.Commit(r.Context()); err!=nil { fail(w,500,"DATABASE_ERROR",err.Error()); return }
    respond(w,200,map[string]string{"order_id":id,"driver_id":driverID,"status":result})
}

func (a *app) dispatchOutbox(ctx context.Context) {
    ticker:=time.NewTicker(500*time.Millisecond); defer ticker.Stop()
    for {
        select { case <-ctx.Done(): return; case <-ticker.C: }
        var id int64
        var orderID,signal string
        var payload []byte
        err:=a.db.QueryRow(ctx,`
            UPDATE driver_signal_outbox
            SET locked_until=NOW()+INTERVAL '30 seconds',attempts=attempts+1
            WHERE outbox_id=(
                SELECT outbox_id FROM driver_signal_outbox
                WHERE dispatched_at IS NULL AND (locked_until IS NULL OR locked_until<NOW())
                ORDER BY outbox_id LIMIT 1 FOR UPDATE SKIP LOCKED
            )
            RETURNING outbox_id,order_id,signal_name,payload`).Scan(&id,&orderID,&signal,&payload)
        if errors.Is(err,pgx.ErrNoRows) { continue }
        if err!=nil { log.Printf("driver outbox claim: %v",err); continue }
        req,err:=http.NewRequestWithContext(ctx,http.MethodPost,a.orderAPI+"/internal/orders/"+orderID+"/signals/"+signal,strings.NewReader(string(payload)))
        if err==nil {
            req.Header.Set("Content-Type","application/json"); req.Header.Set("X-Service-Token",a.token)
            client:=http.Client{Timeout:5*time.Second}
            var response *http.Response
            response,err=client.Do(req)
            if err==nil { response.Body.Close(); if response.StatusCode/100!=2 { err=errors.New("order-api returned "+response.Status) } }
        }
        if err!=nil {
            _,_=a.db.Exec(ctx,"UPDATE driver_signal_outbox SET locked_until=NOW()+make_interval(secs => LEAST(POWER(2,LEAST(attempts,6)),60)),last_error=$2 WHERE outbox_id=$1",id,err.Error())
            log.Printf("driver outbox %d retry: %v",id,err); continue
        }
        _,err=a.db.Exec(ctx,"UPDATE driver_signal_outbox SET dispatched_at=NOW(),locked_until=NULL,last_error='' WHERE outbox_id=$1",id)
        if err!=nil { log.Printf("driver outbox acknowledge %d: %v",id,err) }
    }
}

func (a *app) authorized(r *http.Request) bool { return r.Header.Get("X-Service-Token")==a.token }
func migrate(db *pgxpool.Pool) error {
    data,err:=os.ReadFile("migrations/001_driver_ops.sql"); if err!=nil { return err }
    for _,stmt:=range strings.Split(string(data),";") { stmt=strings.TrimSpace(stmt); if stmt!="" { if _,err=db.Exec(context.Background(),stmt); err!=nil { return err } } }
    return nil
}
func respond(w http.ResponseWriter,status int,v any) { w.Header().Set("Content-Type","application/json"); w.WriteHeader(status); _=json.NewEncoder(w).Encode(v) }
func fail(w http.ResponseWriter,status int,code,message string) { respond(w,status,map[string]string{"error":code,"message":message}) }
func env(k,d string) string { if v:=os.Getenv(k); v!="" { return v }; return d }
