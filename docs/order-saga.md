# Order Saga: Execution and Failure Semantics

## Why a saga

An order spans independent transactional owners. There is no cross-service SQL transaction: Restaurant Ops owns the restaurant ticket, Driver Ops owns driver availability/assignment, Order API owns the customer order, and Temporal persists the orchestration history. The workflow coordinates local transactions and compensating commands.

## Successful path

1. **Create order** — Order API validates the request, requires an `Idempotency-Key`, stores `PENDING`, writes an outbox row in the same PostgreSQL transaction, and returns `202 Accepted`.
2. **Start workflow** — the Order API outbox dispatcher starts `OrderWorkflow` with deterministic ID `order-{order_id}`. Replayed outbox rows cannot create a second execution.
3. **Create restaurant ticket** — Temporal activity calls Restaurant Ops. A compensation is registered before the activity because a network timeout can happen after the downstream commit.
4. **Accept/reject** — Restaurant Ops persists the decision and an outbox event atomically. Its dispatcher calls Order API, which signals the workflow. The workflow waits on a durable signal with a three-minute deadline.
5. **Assign driver** — Driver Ops transactionally locks one available driver using `FOR UPDATE SKIP LOCKED`, creates the order assignment and marks the driver assigned. The command is safe to retry by order ID.
6. **Prepare** — workflow waits up to 45 minutes for `restaurant_ready`.
7. **Pickup** — Driver Ops persists pickup and emits `driver_picked_up`; workflow waits up to 15 minutes.
8. **Delivery** — Driver Ops persists delivery, returns the driver to the available pool and emits `delivery_completed`; workflow waits up to 90 minutes and marks the order `DELIVERED`.

## Compensation order

Compensations execute in reverse order and have their own retry policy:

| Forward action | Compensation | Idempotent behavior |
|---|---|---|
| Create restaurant ticket | Cancel restaurant ticket | Missing ticket is a no-op |
| Assign driver | Release assignment | Missing/released assignment is a no-op |
| Assignment already picked up | Delivery exception | Driver is not incorrectly returned to AVAILABLE |

A rejected or timed-out restaurant order cancels the ticket. A failure after assignment releases the driver first, then cancels the restaurant ticket. When the driver has already picked up food, release marks the assignment and driver `DELIVERY_EXCEPTION`; an operator must resolve the physical delivery. The workflow does not pretend that a courier carrying food can be safely made available again.

Compensation activities use unlimited Temporal retries. The workflow reports `COMPENSATION_FAILED` only if the activity ultimately fails under a configured non-retryable policy or worker/operator intervention. Do not add a maximum retry count to irreversible cleanup without an escalation/reconciliation process.

## Reliability patterns

- **Transactional outbox**: Order API stores order + workflow-start command atomically. Restaurant Ops and Driver Ops store state changes + signal events atomically.
- **At-least-once delivery**: outbox dispatchers may deliver more than once. Workflow signal handlers are idempotent; service commands use order ID as the idempotency key.
- **Activity retries**: only activities perform HTTP/database I/O. Retry policies distinguish bounded forward progress from persistent compensation.
- **Workflow timers**: acceptance, preparation, pickup and delivery deadlines survive worker restarts because they are Temporal timers.
- **Cancellation**: customer cancellation enters the same outbox as workflow-start commands, so it is delivered after the workflow start event. The workflow checks cancellation between stages and compensates completed side effects.
- **Status history**: Order API appends a status event whenever the canonical order state changes.

## Run locally

```bash
cp .env.example .env
docker compose up --build
```

Temporal UI: http://localhost:8088

Create an order (amount is in minor currency units, e.g. paise for INR):

```bash
curl -i http://localhost:8086/v1/orders \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: demo-order-001' \
  -d '{
    "customer_id": "customer-1",
    "restaurant_id": "restaurant-1",
    "items": [{"item_id":"dish-1","quantity":2}],
    "delivery_address": {"line1":"MG Road","city":"Bengaluru"},
    "total_amount_minor": 45000,
    "currency": "INR"
  }'
```

Accept and advance the order from the Restaurant Ops API:

```bash
TOKEN=dev-internal-token
ORDER_ID=<returned-order-id>
curl -X POST http://localhost:8084/v1/restaurant-orders/$ORDER_ID/accept -H "X-Service-Token: $TOKEN"
curl -X POST http://localhost:8084/v1/restaurant-orders/$ORDER_ID/ready -H "X-Service-Token: $TOKEN"
```

For local testing, explicitly seed demo drivers once with `psql "$POSTGRES_DSN" -f infra/postgres/seeds/dev_drivers.sql` (or use your local PostgreSQL connection). The Driver Ops service never creates sample drivers at startup. Select a registered driver in the Driver portal, then pick up and deliver:

```bash
curl -X POST http://localhost:8085/v1/assignments/$ORDER_ID/picked-up -H "X-Service-Token: $TOKEN"
curl -X POST http://localhost:8085/v1/assignments/$ORDER_ID/delivered -H "X-Service-Token: $TOKEN"
curl http://localhost:8086/v1/orders/$ORDER_ID
```

Reject or cancel an order to exercise compensation:

```bash
curl -X POST http://localhost:8084/v1/restaurant-orders/$ORDER_ID/reject -H "X-Service-Token: $TOKEN"
# or
curl -X POST http://localhost:8086/v1/orders/$ORDER_ID/cancel \
  -H 'Content-Type: application/json' -d '{"reason":"customer_changed_mind"}'
```

## Current scope and next production steps

The scaffold includes the core saga, transactional outboxes, PostgreSQL-backed service state and workflow tests. It intentionally does not charge a payment method or issue a real refund. Before production, add a Payment service and register `authorize_payment` / `refund_payment` activities and compensations; add authenticated restaurant/driver-facing identities, proper database-per-service provisioning, event reconciliation/alerting, and integration tests against Compose. Payment authorization should normally happen before confirming the customer order, with refund compensation only after a successful capture/charge.
