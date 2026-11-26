package main

import (
    "context"
    "crypto/rand"
    "crypto/sha256"
    "encoding/hex"
    "encoding/json"
    "errors"
    "fmt"
    "log"
    "net/http"
    "os"
    "strings"
    "time"

    "github.com/segmentio/kafka-go"
    "github.com/jackc/pgx/v5"
    "github.com/jackc/pgx/v5/pgxpool"
    "go.temporal.io/api/enums/v1"
    temporalclient "go.temporal.io/sdk/client"
)

type app struct {
    db *pgxpool.Pool
    temporal temporalclient.Client
    internalToken string
}
type createRequest struct {
    CustomerID string `json:"customer_id"`
    RestaurantID string `json:"restaurant_id"`
    Items json.RawMessage `json:"items"`
    DeliveryAddress json.RawMessage `json:"delivery_address"`
    TotalAmountMinor int64 `json:"total_amount_minor"`
    Currency string `json:"currency"`
}
type workflowInput struct {
    OrderID string `json:"order_id"`
    CustomerID string `json:"customer_id"`
    RestaurantID string `json:"restaurant_id"`
    Items json.RawMessage `json:"items"`
    DeliveryAddress json.RawMessage `json:"delivery_address"`
    TotalAmountMinor int64 `json:"total_amount_minor"`
    Currency string `json:"currency"`
}
type statusRequest struct {
    Status string `json:"status"`
    Reason string `json:"reason"`
}

var terminal = map[string]bool{"DELIVERED":true,"REJECTED":true,"CANCELLED":true,"FAILED":true,"COMPENSATION_FAILED":true}
var validStatus = map[string]bool{
    "PENDING":true,"AWAITING_RESTAURANT_ACCEPTANCE":true,"RESTAURANT_ACCEPTED":true,
    "DRIVER_ASSIGNED":true,"PREPARING":true,"READY_FOR_PICKUP":true,"OUT_FOR_DELIVERY":true,
    "DELIVERED":true,"REJECTED":true,"CANCELLED":true,"FAILED":true,"COMPENSATION_FAILED":true,
}

func main() {
    ctx:=context.Background()
    db,err:=pgxpool.New(ctx,env("DATABASE_URL","postgres://food:food@localhost:5432/food_delivery?sslmode=disable"))
    if err!=nil { log.Fatal(err) }; defer db.Close()
    if err=db.Ping(ctx); err!=nil { log.Fatal(err) }
    if err=migrate(db); err!=nil { log.Fatal(err) }

    address:=env("TEMPORAL_ADDRESS","localhost:7233")
    var temporal temporalclient.Client
    for {
        temporal,err=temporalclient.Dial(temporalclient.Options{HostPort:address})
        if err==nil { break }
        log.Printf("waiting for Temporal at %s: %v",address,err)
        time.Sleep(3*time.Second)
    }
    defer temporal.Close()
    a:=&app{db:db,temporal:temporal,internalToken:env("INTERNAL_SERVICE_TOKEN","dev-internal-token")}
    go a.consumeOrderOutbox(ctx)

    mux:=http.NewServeMux()
    mux.HandleFunc("GET /healthz",a.health)
    mux.HandleFunc("POST /v1/orders",a.createOrder)
    mux.HandleFunc("GET /v1/orders/{id}",a.getOrder)
    mux.HandleFunc("GET /v1/orders/{id}/events",a.orderEvents)
    mux.HandleFunc("POST /v1/orders/{id}/cancel",a.cancelOrder)
    mux.HandleFunc("POST /internal/orders/{id}/signals/{signal}",a.signalWorkflow)
    mux.HandleFunc("PATCH /internal/orders/{id}/status",a.updateStatus)

    port:=env("PORT","8086")
    log.Printf("order-api listening on :%s",port)
    log.Fatal((&http.Server{Addr:":"+port,Handler:mux,ReadHeaderTimeout:5*time.Second}).ListenAndServe())
}

func (a *app) health(w http.ResponseWriter,r *http.Request) {
    ctx,cancel:=context.WithTimeout(r.Context(),2*time.Second); defer cancel()
    if err:=a.db.Ping(ctx); err!=nil { fail(w,503,"DATABASE_UNAVAILABLE",err.Error()); return }
    respond(w,200,map[string]string{"service":"order-api","status":"ok"})
}

func (a *app) createOrder(w http.ResponseWriter,r *http.Request) {
    var req createRequest
    if err:=json.NewDecoder(http.MaxBytesReader(w,r.Body,1<<20)).Decode(&req); err!=nil { fail(w,400,"INVALID_JSON",err.Error()); return }
    if req.CustomerID=="" || req.RestaurantID=="" || len(req.Items)==0 || !json.Valid(req.Items) || req.TotalAmountMinor<0 {
        fail(w,400,"VALIDATION_ERROR","customer_id, restaurant_id, valid items and non-negative total_amount_minor are required"); return
    }
    key:=strings.TrimSpace(r.Header.Get("Idempotency-Key"))
    if key=="" { fail(w,400,"IDEMPOTENCY_KEY_REQUIRED","send an Idempotency-Key header"); return }
    if req.Currency=="" { req.Currency="INR" }
    if len(req.DeliveryAddress)==0 { req.DeliveryAddress=json.RawMessage(`{}`) }

    // Hash canonical decoded JSON so a key cannot silently be reused for a
    // different order payload.
    var canonicalItems any
    if err:=json.Unmarshal(req.Items,&canonicalItems); err!=nil { fail(w,400,"INVALID_ITEMS",err.Error()); return }
    canonical,_:=json.Marshal(struct{
        CustomerID string
        RestaurantID string
        Items any
        Address json.RawMessage
        Amount int64
        Currency string
    }{req.CustomerID,req.RestaurantID,canonicalItems,req.DeliveryAddress,req.TotalAmountMinor,req.Currency})
    sum:=sha256.Sum256(canonical)
    fingerprint:=hex.EncodeToString(sum[:])

    orderID,err:=newID()
    if err!=nil { fail(w,500,"ID_GENERATION_FAILED",err.Error()); return }
    input:=workflowInput{orderID,req.CustomerID,req.RestaurantID,req.Items,req.DeliveryAddress,req.TotalAmountMinor,req.Currency}
    payload,_:=json.Marshal(input)

    tx,err:=a.db.Begin(r.Context())
    if err!=nil { fail(w,500,"DATABASE_ERROR",err.Error()); return }
    defer tx.Rollback(r.Context())
    var inserted string
    err=tx.QueryRow(r.Context(),`
        INSERT INTO food_orders(order_id,idempotency_key,request_hash,customer_id,restaurant_id,items,delivery_address,total_amount_minor,currency)
        VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)
        ON CONFLICT(idempotency_key) DO NOTHING RETURNING order_id`,
        orderID,key,fingerprint,req.CustomerID,req.RestaurantID,req.Items,req.DeliveryAddress,req.TotalAmountMinor,req.Currency,
    ).Scan(&inserted)
    if errors.Is(err,pgx.ErrNoRows) {
        var oldHash string
        if err=tx.QueryRow(r.Context(),"SELECT order_id,request_hash FROM food_orders WHERE idempotency_key=$1",key).Scan(&inserted,&oldHash); err!=nil {
            fail(w,500,"DATABASE_ERROR",err.Error()); return
        }
        if err=tx.Commit(r.Context()); err!=nil { fail(w,500,"DATABASE_ERROR",err.Error()); return }
        if oldHash!=fingerprint { fail(w,409,"IDEMPOTENCY_KEY_REUSED","Idempotency-Key was already used with a different payload"); return }
        a.returnOrder(w,r,inserted,200)
        return
    }
    if err!=nil { fail(w,500,"DATABASE_ERROR",err.Error()); return }
    _,err=tx.Exec(r.Context(),`INSERT INTO food_order_events(order_id,from_status,to_status,reason) VALUES($1,'','PENDING','order_created')`,inserted)
    if err!=nil { fail(w,500,"DATABASE_ERROR",err.Error()); return }
    eventPayload,_:=json.Marshal(map[string]any{"event_type":"START_ORDER_WORKFLOW","workflow_input":json.RawMessage(payload)})
    _,err=tx.Exec(r.Context(),`INSERT INTO food_order_outbox(event_type,aggregatetype,aggregate_id,payload) VALUES('START_ORDER_WORKFLOW','food.order.events',$1,$2)`,inserted,eventPayload)
    if err!=nil { fail(w,500,"OUTBOX_ERROR",err.Error()); return }
    if err=tx.Commit(r.Context()); err!=nil { fail(w,500,"DATABASE_ERROR",err.Error()); return }
    a.returnOrder(w,r,inserted,http.StatusAccepted)
}

func (a *app) getOrder(w http.ResponseWriter,r *http.Request) { a.returnOrder(w,r,r.PathValue("id"),200) }

func (a *app) returnOrder(w http.ResponseWriter,r *http.Request,id string,status int) {
    var orderID,customerID,restaurantID,currency,state,reason string
    var items,address []byte
    var total int64
    var created,updated time.Time
    err:=a.db.QueryRow(r.Context(),`
        SELECT order_id,customer_id,restaurant_id,items,delivery_address,total_amount_minor,currency,status,failure_reason,created_at,updated_at
        FROM food_orders WHERE order_id=$1`,id).Scan(
        &orderID,&customerID,&restaurantID,&items,&address,&total,&currency,&state,&reason,&created,&updated,
    )
    if errors.Is(err,pgx.ErrNoRows) { fail(w,404,"ORDER_NOT_FOUND","order does not exist"); return }
    if err!=nil { fail(w,500,"DATABASE_ERROR",err.Error()); return }
    var decodedItems,decodedAddress any
    if err=json.Unmarshal(items,&decodedItems); err!=nil { fail(w,500,"DATA_CORRUPTION",err.Error()); return }
    if err=json.Unmarshal(address,&decodedAddress); err!=nil { fail(w,500,"DATA_CORRUPTION",err.Error()); return }
    respond(w,status,map[string]any{
        "order_id":orderID,"customer_id":customerID,"restaurant_id":restaurantID,
        "items":decodedItems,"delivery_address":decodedAddress,"total_amount_minor":total,
        "currency":currency,"status":state,"failure_reason":reason,"created_at":created,"updated_at":updated,
    })
}

func (a *app) cancelOrder(w http.ResponseWriter,r *http.Request) {
    id:=r.PathValue("id")
    var body struct { Reason string `json:"reason"` }
    _=json.NewDecoder(r.Body).Decode(&body)
    if body.Reason=="" { body.Reason="cancelled_by_customer" }
    tx,err:=a.db.Begin(r.Context())
    if err!=nil { fail(w,500,"DATABASE_ERROR",err.Error()); return }
    defer tx.Rollback(r.Context())
    var status string
    err=tx.QueryRow(r.Context(),"SELECT status FROM food_orders WHERE order_id=$1 FOR UPDATE",id).Scan(&status)
    if errors.Is(err,pgx.ErrNoRows) { fail(w,404,"ORDER_NOT_FOUND","order does not exist"); return }
    if err!=nil { fail(w,500,"DATABASE_ERROR",err.Error()); return }
    if terminal[status] { fail(w,409,"ORDER_TERMINAL","order is already "+status); return }
    payload,_:=json.Marshal(map[string]any{"event_type":"SIGNAL_CANCEL","signal":map[string]string{"reason":body.Reason}})
    _,err=tx.Exec(r.Context(),`INSERT INTO food_order_outbox(event_type,aggregatetype,aggregate_id,payload) VALUES('SIGNAL_CANCEL','food.order.events',$1,$2)`,id,payload)
    if err!=nil { fail(w,500,"OUTBOX_ERROR",err.Error()); return }
    if err=tx.Commit(r.Context()); err!=nil { fail(w,500,"DATABASE_ERROR",err.Error()); return }
    respond(w,202,map[string]string{"order_id":id,"status":"CANCEL_REQUESTED"})
}

func (a *app) signalWorkflow(w http.ResponseWriter,r *http.Request) {
    if !a.authorized(r) { fail(w,401,"UNAUTHORIZED","invalid service token"); return }
    signal:=r.PathValue("signal")
    allowed:=map[string]bool{"restaurant_decision":true,"restaurant_ready":true,"driver_picked_up":true,"delivery_completed":true}
    if !allowed[signal] { fail(w,400,"UNKNOWN_SIGNAL","unsupported workflow signal"); return }
    var payload map[string]any
    if err:=json.NewDecoder(r.Body).Decode(&payload); err!=nil { payload=map[string]any{} }
    if err:=a.temporal.SignalWorkflow(r.Context(),"order-"+r.PathValue("id"),"",signal,payload); err!=nil {
        // Restaurant/driver resources are created by an already-started workflow.
        // A late outbox retry after the workflow closed is therefore a stale event,
        // not a reason to keep the source service's outbox row retrying forever.
        if workflowClosed(err) {
            respond(w,202,map[string]string{"signal":signal,"accepted":"true","ignored":"workflow already closed"})
            return
        }
        fail(w,503,"WORKFLOW_SIGNAL_FAILED",err.Error()); return
    }
    respond(w,202,map[string]string{"signal":signal,"accepted":"true"})
}

func (a *app) updateStatus(w http.ResponseWriter,r *http.Request) {
    if !a.authorized(r) { fail(w,401,"UNAUTHORIZED","invalid service token"); return }
    var req statusRequest
    if err:=json.NewDecoder(r.Body).Decode(&req); err!=nil { fail(w,400,"INVALID_JSON",err.Error()); return }
    if !validStatus[req.Status] { fail(w,400,"INVALID_STATUS","unsupported order status"); return }
    tx,err:=a.db.Begin(r.Context())
    if err!=nil { fail(w,500,"DATABASE_ERROR",err.Error()); return }
    defer tx.Rollback(r.Context())
    var current string
    err=tx.QueryRow(r.Context(),"SELECT status FROM food_orders WHERE order_id=$1 FOR UPDATE",r.PathValue("id")).Scan(&current)
    if errors.Is(err,pgx.ErrNoRows) { fail(w,404,"ORDER_NOT_FOUND","order does not exist"); return }
    if err!=nil { fail(w,500,"DATABASE_ERROR",err.Error()); return }
    if terminal[current] && current!=req.Status { fail(w,409,"ORDER_TERMINAL","cannot transition "+current+" to "+req.Status); return }
    if current!=req.Status {
        _,err=tx.Exec(r.Context(),`UPDATE food_orders SET status=$2,failure_reason=$3,updated_at=NOW() WHERE order_id=$1`,r.PathValue("id"),req.Status,req.Reason)
        if err!=nil { fail(w,500,"DATABASE_ERROR",err.Error()); return }
        _,err=tx.Exec(r.Context(),`INSERT INTO food_order_events(order_id,from_status,to_status,reason) VALUES($1,$2,$3,$4)`,r.PathValue("id"),current,req.Status,req.Reason)
        if err!=nil { fail(w,500,"DATABASE_ERROR",err.Error()); return }
    }
    if err=tx.Commit(r.Context()); err!=nil { fail(w,500,"DATABASE_ERROR",err.Error()); return }
    respond(w,200,map[string]string{"order_id":r.PathValue("id"),"status":req.Status})
}

type orderOutboxEvent struct {
    EventType string `json:"event_type"`
    WorkflowInput workflowInput `json:"workflow_input"`
    Signal map[string]string `json:"signal"`
}

// consumeOrderOutbox reads committed rows emitted by Debezium's Outbox Event Router.
// Kafka offsets are committed only after Temporal accepts the corresponding operation.
func (a *app) consumeOrderOutbox(ctx context.Context) {
    reader:=kafka.NewReader(kafka.ReaderConfig{
        Brokers:strings.Split(env("KAFKA_BROKERS","kafka:29092"),","),
        Topic:env("KAFKA_ORDER_EVENTS_TOPIC","food.order.events"),
        GroupID:env("KAFKA_CONSUMER_GROUP","order-api-temporal"),
        MinBytes:1,MaxBytes:10e6,CommitInterval:0,
    })
    defer reader.Close()
    log.Printf("Debezium order outbox consumer started")
    for {
        msg,err:=reader.FetchMessage(ctx)
        if err!=nil { if ctx.Err()!=nil { return }; log.Printf("Kafka fetch: %v",err); time.Sleep(time.Second); continue }
        var event orderOutboxEvent
        if err=json.Unmarshal(msg.Value,&event); err!=nil { log.Printf("invalid outbox event at offset %d: %v",msg.Offset,err); time.Sleep(time.Second); continue }
        if err=a.handleOrderOutboxEvent(ctx,msg.Key,event); err!=nil { log.Printf("outbox event %s retry: %v",event.EventType,err); time.Sleep(time.Second); continue }
        if err=reader.CommitMessages(ctx,msg); err!=nil { log.Printf("Kafka offset commit: %v",err) }
    }
}

func (a *app) handleOrderOutboxEvent(ctx context.Context,key []byte,event orderOutboxEvent) error {
    switch event.EventType {
    case "START_ORDER_WORKFLOW":
        input:=event.WorkflowInput
        if input.OrderID=="" { return fmt.Errorf("workflow_input.order_id is required") }
        startCtx,cancel:=context.WithTimeout(ctx,10*time.Second); defer cancel()
        _,err:=a.temporal.ExecuteWorkflow(startCtx,temporalclient.StartWorkflowOptions{
            ID:"order-"+input.OrderID,TaskQueue:env("TEMPORAL_TASK_QUEUE","food-delivery-orders"),
            WorkflowIDReusePolicy:enums.WORKFLOW_ID_REUSE_POLICY_REJECT_DUPLICATE,
        },"OrderWorkflow",input)
        if err!=nil { msg:=strings.ToLower(err.Error()); if strings.Contains(msg,"already started") || strings.Contains(msg,"already exists") { return nil } }
        return err
    case "SIGNAL_CANCEL":
        orderID:=string(key); if orderID=="" { return fmt.Errorf("Kafka key must contain order id") }
        err:=a.temporal.SignalWorkflow(ctx,"order-"+orderID,"","cancel_order",event.Signal)
        if err!=nil && workflowClosed(err) { return nil }; return err
    default: return fmt.Errorf("unknown order outbox event type %q",event.EventType)
    }
}
func (a *app) authorized(r *http.Request) bool { return r.Header.Get("X-Service-Token")==a.internalToken }
func migrate(db *pgxpool.Pool) error {
    data,err:=os.ReadFile("migrations/001_orders.sql"); if err!=nil { return err }
    for _,statement:=range strings.Split(string(data),";") {
        statement=strings.TrimSpace(statement)
        if statement!="" { if _,err=db.Exec(context.Background(),statement); err!=nil { return err } }
    }
    return nil
}
func newID() (string,error) {
    b:=make([]byte,16); if _,err:=rand.Read(b); err!=nil { return "",err }
    b[6]=(b[6]&0x0f)|0x40; b[8]=(b[8]&0x3f)|0x80
    h:=hex.EncodeToString(b)
    return fmt.Sprintf("%s-%s-%s-%s-%s",h[:8],h[8:12],h[12:16],h[16:20],h[20:]),nil
}
func respond(w http.ResponseWriter,status int,v any) { w.Header().Set("Content-Type","application/json"); w.WriteHeader(status); _=json.NewEncoder(w).Encode(v) }
func fail(w http.ResponseWriter,status int,code,message string) { respond(w,status,map[string]string{"error":code,"message":message}) }
func env(k,d string) string { if v:=os.Getenv(k); v!="" { return v }; return d }

func workflowClosed(err error) bool {
    message:=strings.ToLower(err.Error())
    return strings.Contains(message,"not found") ||
        strings.Contains(message,"already completed") ||
        strings.Contains(message,"workflow execution has completed")
}
