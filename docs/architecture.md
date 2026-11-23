# Architecture

## Bounded contexts and ownership

- **User/Auth**: identity, credentials, access tokens and customer profiles. PostgreSQL.
- **Catalog**: restaurant/menu data and Elasticsearch read index.
- **Search**: query-only API over Elasticsearch; catalog owns document updates.
- **Restaurant Ops**: restaurant order acceptance, rejection, preparation, readiness and cancellation. PostgreSQL.
- **Driver Ops**: driver availability, assignment, pickup, delivery and exception handling. PostgreSQL.
- **Order API**: customer orders, idempotency, status history, workflow start and internal signal ingress. PostgreSQL.
- **Temporal worker**: durable order saga; Python SDK. It contains orchestration and no direct I/O.

## Saga sequence

```mermaid
sequenceDiagram
  autonumber
  participant C as Customer
  participant O as Order API
  participant P as PostgreSQL
  participant T as Temporal
  participant R as Restaurant Ops
  participant D as Driver Ops
  C->>O: POST /v1/orders + Idempotency-Key
  O->>P: INSERT order + START outbox (one transaction)
  O-->>C: 202 Accepted + order_id
  O->>T: Start OrderWorkflow (outbox dispatcher)
  T->>R: Create restaurant ticket (Activity)
  R->>P: Persist ticket
  R-->>T: Activity result
  T-->>T: Wait for restaurant_decision signal
  R->>P: Accept/reject + signal outbox (one transaction)
  R->>O: POST internal signal callback
  O->>T: Signal restaurant_decision
  T->>D: Assign driver (Activity)
  D->>P: Lock driver + create assignment
  T-->>T: Wait restaurant_ready, driver_picked_up, delivery_completed
  R->>O: restaurant_ready signal (outbox relay)
  D->>O: pickup/delivery signals (outbox relay)
  T->>O: Update canonical order status (Activity)
  O-->>C: GET /v1/orders/{id}
  Note over T,R,D: Failure or cancellation runs compensations in reverse order
```

## Order saga

See [order-saga.md](order-saga.md) for detailed sequence, commands, timeouts, compensation semantics and a runnable walkthrough.

Temporal workflow code must be deterministic: no network, database, random or wall-clock calls inside workflow functions. All side effects are activities. Signals record restaurant and driver events in workflow history. The workflow uses server-side timers for acceptance, preparation, pickup and delivery timeouts.

## Data and messaging

PostgreSQL owns transactional state. Temporal's PostgreSQL schema and `default` namespace are initialized by the pinned Temporal admin-tools image before the server-dependent services start. Each application service owns its own migrations and uniquely named tables in the development database. For production, provision a database/schema and role per service. Elasticsearch is a derived catalog/search index, not operational truth. Redis is reserved for ephemeral cache/rate limiting/geo lookup. Kafka is available for broader domain-event fan-out; the current order saga uses transactional outboxes for reliable Temporal handoff.

## Reliability

- Order API persists an order and its workflow-start outbox event in one transaction.
- Restaurant Ops and Driver Ops persist business state and signal outbox records in the same transaction.
- Outbox delivery is at least once; Temporal signal handlers and service operations are idempotent.
- Forward activities use bounded retries. Compensation activities use persistent retries.
- Status events provide an audit trail.
- A driver who has picked up food is not returned to the available pool by compensation; it is marked for operational exception handling.
