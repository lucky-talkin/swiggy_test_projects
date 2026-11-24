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
type createRequest struct {
    OrderID string `json:"order_id"`
    RestaurantID string `json:"restaurant_id"`
    CustomerID string `json:"customer_id"`
    Items json.RawMessage `json:"items"`
}

func main() {
    ctx:=context.Background()
    db,err:=pgxpool.New(ctx,env("DATABASE_URL","postgres://food:food@localhost:5432/food_delivery?sslmode=disable"))
    if err!=nil { log.Fatal(err) }; defer db.Close()
    if err=db.Ping(ctx); err!=nil { log.Fatal(err) }
    if err=migrate(db); err!=nil { log.Fatal(err) }
    a:=&app{db:db,orderAPI:env("ORDER_API_URL","http://localhost:8086"),token:env("INTERNAL_SERVICE_TOKEN","dev-internal-token")}
    go a.dispatchOutbox(ctx)

    mux:=http.NewServeMux()
    mux.HandleFunc("GET /healthz",a.health)
    mux.HandleFunc("POST /v1/restaurant-orders",a.create)
    mux.HandleFunc("POST /v1/restaurant-orders/{order_id}/accept",func(w http.ResponseWriter,r *http.Request){ a.transition(w,r,"ACCEPTED","restaurant_decision",map[string]string{"decision":"ACCEPTED"}) })
    mux.HandleFunc("POST /v1/restaurant-orders/{order_id}/reject",func(w http.ResponseWriter,r *http.Request){ a.transition(w,r,"REJECTED","restaurant_decision",map[string]string{"decision":"REJECTED"}) })
    mux.HandleFunc("POST /v1/restaurant-orders/{order_id}/ready",func(w http.ResponseWriter,r *http.Request){ a.transition(w,r,"READY","restaurant_ready",map[string]string{"state":"READY"}) })
    mux.HandleFunc("POST /v1/restaurant-orders/{order_id}/cancel",a.cancel)
    a.registerPortalRoutes(mux)
    port:=env("PORT","8084")
    log.Printf("restaurant-ops listening on :%s",port)
    log.Fatal((&http.Server{Addr:":"+port,Handler:mux,ReadHeaderTimeout:5*time.Second}).ListenAndServe())
}

func (a *app) health(w http.ResponseWriter,r *http.Request) {
    ctx,cancel:=context.WithTimeout(r.Context(),2*time.Second); defer cancel()
    if err:=a.db.Ping(ctx); err!=nil { fail(w,503,"DATABASE_UNAVAILABLE",err.Error()); return }
    respond(w,200,map[string]string{"service":"restaurant-ops","status":"ok"})
}

func (a *app) create(w http.ResponseWriter,r *http.Request) {
    if !a.authorized(r) { fail(w,401,"UNAUTHORIZED","invalid service token"); return }
    var req createRequest
    if err:=json.NewDecoder(http.MaxBytesReader(w,r.Body,1<<20)).Decode(&req); err!=nil { fail(w,400,"INVALID_JSON",err.Error()); return }
    if req.OrderID=="" || req.RestaurantID=="" { fail(w,400,"VALIDATION_ERROR","order_id and restaurant_id are required"); return }
    var accepting bool
    settingsErr:=a.db.QueryRow(r.Context(),"SELECT accepting_orders FROM restaurant_settings WHERE restaurant_id=$1",req.RestaurantID).Scan(&accepting)
    if settingsErr!=nil && !errors.Is(settingsErr,pgx.ErrNoRows) { fail(w,500,"DATABASE_ERROR",settingsErr.Error()); return }
    if settingsErr==nil && !accepting { fail(w,409,"RESTAURANT_NOT_ACCEPTING","restaurant is not accepting orders"); return }
    if len(req.Items)==0 { req.Items=json.RawMessage(`[]`) }
    if !json.Valid(req.Items) { fail(w,400,"VALIDATION_ERROR","items must be valid JSON"); return }
    _,err:=a.db.Exec(r.Context(),`
        INSERT INTO restaurant_orders(order_id,restaurant_id,customer_id,items)
        VALUES($1,$2,$3,$4) ON CONFLICT(order_id) DO NOTHING`,
        req.OrderID,req.RestaurantID,req.CustomerID,req.Items)
    if err!=nil { fail(w,500,"DATABASE_ERROR",err.Error()); return }
    respond(w,200,map[string]string{"order_id":req.OrderID,"status":"PENDING"})
}

func (a *app) transition(w http.ResponseWriter,r *http.Request,target,signal string,payload map[string]string) {
    if !a.authorized(r) { fail(w,401,"UNAUTHORIZED","invalid service token"); return }
    id:=r.PathValue("order_id")
    tx,err:=a.db.Begin(r.Context()); if err!=nil { fail(w,500,"DATABASE_ERROR",err.Error()); return }
    defer tx.Rollback(r.Context())
    var current string
    err=tx.QueryRow(r.Context(),"SELECT status FROM restaurant_orders WHERE order_id=$1 FOR UPDATE",id).Scan(&current)
    if errors.Is(err,pgx.ErrNoRows) { fail(w,404,"RESTAURANT_ORDER_NOT_FOUND","restaurant order does not exist"); return }
    if err!=nil { fail(w,500,"DATABASE_ERROR",err.Error()); return }

    valid:=false
    switch target {
    case "ACCEPTED","REJECTED": valid=current=="PENDING" || current==target
    case "READY": valid=current=="ACCEPTED" || current==target
    }
    if !valid { fail(w,409,"INVALID_RESTAURANT_TRANSITION","cannot transition "+current+" to "+target); return }
    if current!=target {
        _,err=tx.Exec(r.Context(),"UPDATE restaurant_orders SET status=$2,updated_at=NOW() WHERE order_id=$1",id,target)
        if err!=nil { fail(w,500,"DATABASE_ERROR",err.Error()); return }
    }
    body,_:=json.Marshal(payload)
    _,err=tx.Exec(r.Context(),`
        INSERT INTO restaurant_signal_outbox(order_id,signal_name,payload) VALUES($1,$2,$3)`,id,signal,body)
    if err!=nil { fail(w,500,"OUTBOX_ERROR",err.Error()); return }
    if err=tx.Commit(r.Context()); err!=nil { fail(w,500,"DATABASE_ERROR",err.Error()); return }
    respond(w,202,map[string]string{"order_id":id,"status":target,"workflow_signal":"QUEUED"})
}

func (a *app) cancel(w http.ResponseWriter,r *http.Request) {
    if !a.authorized(r) { fail(w,401,"UNAUTHORIZED","invalid service token"); return }
    id:=r.PathValue("order_id")
    tx,err:=a.db.Begin(r.Context()); if err!=nil { fail(w,500,"DATABASE_ERROR",err.Error()); return }
    defer tx.Rollback(r.Context())
    var current string
    err=tx.QueryRow(r.Context(),"SELECT status FROM restaurant_orders WHERE order_id=$1 FOR UPDATE",id).Scan(&current)
    if errors.Is(err,pgx.ErrNoRows) { _=tx.Commit(r.Context()); respond(w,200,map[string]string{"order_id":id,"status":"NOT_CREATED"}); return }
    if err!=nil { fail(w,500,"DATABASE_ERROR",err.Error()); return }
    if current!="CANCELLED" && current!="REJECTED" {
        _,err=tx.Exec(r.Context(),"UPDATE restaurant_orders SET status='CANCELLED',updated_at=NOW() WHERE order_id=$1",id)
        if err!=nil { fail(w,500,"DATABASE_ERROR",err.Error()); return }
    }
    if err=tx.Commit(r.Context()); err!=nil { fail(w,500,"DATABASE_ERROR",err.Error()); return }
    respond(w,200,map[string]string{"order_id":id,"status":"CANCELLED"})
}

func (a *app) dispatchOutbox(ctx context.Context) {
    ticker:=time.NewTicker(500*time.Millisecond); defer ticker.Stop()
    for {
        select { case <-ctx.Done(): return; case <-ticker.C: }
        var id int64
        var orderID,signal string
        var payload []byte
        err:=a.db.QueryRow(ctx,`
            UPDATE restaurant_signal_outbox
            SET locked_until=NOW()+INTERVAL '30 seconds',attempts=attempts+1
            WHERE outbox_id=(
                SELECT outbox_id FROM restaurant_signal_outbox
                WHERE dispatched_at IS NULL AND (locked_until IS NULL OR locked_until<NOW())
                ORDER BY outbox_id LIMIT 1 FOR UPDATE SKIP LOCKED
            )
            RETURNING outbox_id,order_id,signal_name,payload`).Scan(&id,&orderID,&signal,&payload)
        if errors.Is(err,pgx.ErrNoRows) { continue }
        if err!=nil { log.Printf("restaurant outbox claim: %v",err); continue }
        req,err:=http.NewRequestWithContext(ctx,http.MethodPost,a.orderAPI+"/internal/orders/"+orderID+"/signals/"+signal,strings.NewReader(string(payload)))
        if err==nil {
            req.Header.Set("Content-Type","application/json"); req.Header.Set("X-Service-Token",a.token)
            client:=http.Client{Timeout:5*time.Second}
            var response *http.Response
            response,err=client.Do(req)
            if err==nil { response.Body.Close(); if response.StatusCode/100!=2 { err=errors.New("order-api returned "+response.Status) } }
        }
        if err!=nil {
            _,_=a.db.Exec(ctx,"UPDATE restaurant_signal_outbox SET locked_until=NOW()+make_interval(secs => LEAST(POWER(2,LEAST(attempts,6)),60)),last_error=$2 WHERE outbox_id=$1",id,err.Error())
            log.Printf("restaurant outbox %d retry: %v",id,err); continue
        }
        _,err=a.db.Exec(ctx,"UPDATE restaurant_signal_outbox SET dispatched_at=NOW(),locked_until=NULL,last_error='' WHERE outbox_id=$1",id)
        if err!=nil { log.Printf("restaurant outbox acknowledge %d: %v",id,err) }
    }
}

func (a *app) authorized(r *http.Request) bool { return r.Header.Get("X-Service-Token")==a.token }
func migrate(db *pgxpool.Pool) error {
    data,err:=os.ReadFile("migrations/001_restaurant_orders.sql"); if err!=nil { return err }
    for _,stmt:=range strings.Split(string(data),";") { stmt=strings.TrimSpace(stmt); if stmt!="" { if _,err=db.Exec(context.Background(),stmt); err!=nil { return err } } }
    return nil
}
func respond(w http.ResponseWriter,status int,v any) { w.Header().Set("Content-Type","application/json"); w.WriteHeader(status); _=json.NewEncoder(w).Encode(v) }
func fail(w http.ResponseWriter,status int,code,message string) { respond(w,status,map[string]string{"error":code,"message":message}) }
func env(k,d string) string { if v:=os.Getenv(k); v!="" { return v }; return d }
