# API contracts

JSON requests require `Content-Type: application/json`. Internal endpoints require `X-Service-Token`, configured by `INTERNAL_SERVICE_TOKEN`. These development defaults are not suitable for production.

## Order API — :8086

- `POST /v1/orders` — create order. Required `Idempotency-Key`; body: `customer_id`, `restaurant_id`, `items`, `total_amount_minor`, optional `delivery_address`, optional `currency`. Returns 202 and order state. A repeated key with identical payload returns the original order; a different payload returns 409.
- `GET /v1/orders/{id}` — fetch state.
- `POST /v1/orders/{id}/cancel` — request cancellation; cancellation is delivered through the order outbox.
- `PATCH /internal/orders/{id}/status` — worker-only canonical status update.
- `POST /internal/orders/{id}/signals/{signal}` — internal Temporal signal ingress.
- `GET /healthz`.

## Restaurant Ops — :8084

- `POST /v1/restaurant-orders` — internal creation of a restaurant ticket.
- `POST /v1/restaurant-orders/{order_id}/accept` — persist acceptance and signal workflow.
- `POST /v1/restaurant-orders/{order_id}/reject` — persist rejection and signal workflow.
- `POST /v1/restaurant-orders/{order_id}/ready` — mark ready and signal workflow.
- `POST /v1/restaurant-orders/{order_id}/cancel` — idempotent compensation.
- `GET /healthz`.

## Driver Ops — :8085

- `POST /v1/drivers/{driver_id}/availability` — set available/offline (internal).
- `POST /v1/assignments` — idempotently assign a free driver to an order.
- `POST /v1/assignments/{order_id}/picked-up` — persist pickup and signal workflow.
- `POST /v1/assignments/{order_id}/delivered` — persist delivery and signal workflow.
- `POST /v1/assignments/{order_id}/release` — idempotent compensation; picked-up assignments become `DELIVERY_EXCEPTION`.
- `GET /healthz`.

## Workflow contract

Workflow ID: `order-{order_id}`; Task Queue: `food-delivery-orders`.

Signals: `restaurant_decision`, `restaurant_ready`, `driver_picked_up`, `delivery_completed`, `cancel_order`.

Statuses: `PENDING → AWAITING_RESTAURANT_ACCEPTANCE → RESTAURANT_ACCEPTED → DRIVER_ASSIGNED → PREPARING → READY_FOR_PICKUP → OUT_FOR_DELIVERY → DELIVERED`. Failure terminals: `REJECTED`, `CANCELLED`, `FAILED`, `COMPENSATION_FAILED`.
