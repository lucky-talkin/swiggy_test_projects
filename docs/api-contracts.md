# API contracts

JSON requests require `Content-Type: application/json`. Internal endpoints require `X-Service-Token`, configured by `INTERNAL_SERVICE_TOKEN`. These development defaults are not suitable for production.

## Order API — :8086

- `POST /v1/orders` — create order. Required `Idempotency-Key`; body: `customer_id`, `restaurant_id`, `items`, `total_amount_minor`, optional `delivery_address`, optional `currency`. Returns 202 and order state. A repeated key with identical payload returns the original order; a different payload returns 409.
- `GET /v1/orders/{id}` — fetch state.
- `GET /v1/orders/{id}/events` — Server-Sent Events stream; emits an `order` event when the canonical order row changes.
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
- `GET /v1/portal/restaurants/{restaurant_id}` — portal profile and accepting-orders state.
- `GET /v1/portal/restaurants/{restaurant_id}/orders` — enriched restaurant queue.
- `GET /v1/portal/restaurants/{restaurant_id}/events` — SSE `refresh` events when canonical order state changes.
- `POST /v1/portal/restaurants/{restaurant_id}/availability` — body `{ "accepting_orders": true|false }`.
- `POST /v1/portal/restaurants/{restaurant_id}/orders/{order_id}/accept|reject|ready` — restaurant decisions; writes an outbox signal for Temporal.
- `GET /healthz`.

## Driver Ops — :8085

- `POST /v1/drivers/{driver_id}/availability` — set available/offline (internal).
- `POST /v1/assignments` — idempotently assign a free driver to an order.
- `POST /v1/assignments/{order_id}/picked-up` — persist pickup and signal workflow.
- `POST /v1/assignments/{order_id}/delivered` — persist delivery and signal workflow.
- `POST /v1/assignments/{order_id}/release` — idempotent compensation; picked-up assignments become `DELIVERY_EXCEPTION`.
- `GET /v1/portal/drivers/{driver_id}` — driver status.
- `GET /v1/portal/drivers/{driver_id}/assignments` — assigned trips joined with canonical order details. Response includes both `status` (order state) and `assignment_status` (driver state).
- `GET /v1/portal/drivers/{driver_id}/events` — SSE `refresh` events on assignment, availability, or order-state changes.
- `POST /v1/portal/drivers/{driver_id}/availability` — body `{ "available": true|false }`.
- `POST /v1/portal/drivers/{driver_id}/assignments/{order_id}/picked-up|delivered` — driver confirmations; writes an outbox signal for Temporal.
- `GET /healthz`.

## Portal access

Portal routes require `X-Portal-Token`. Native browser EventSource cannot attach custom headers, so SSE endpoints also accept a `token` query parameter. The repository's `dev-portal-token` is for local development only; it is shared across roles and is **not** production authentication or tenant isolation. Replace it with authenticated restaurant/driver sessions and derive the restaurant/driver ID from verified token claims before deployment.

SSE uses database polling (1-second change detection) plus 15-second heartbeat comments. Clients must refresh their REST snapshot on `refresh` events and rely on EventSource reconnect behavior.

## Workflow contract

Workflow ID: `order-{order_id}`; Task Queue: `food-delivery-orders`.

Signals: `restaurant_decision`, `restaurant_ready`, `driver_picked_up`, `delivery_completed`, `cancel_order`.

Statuses: `PENDING → AWAITING_RESTAURANT_ACCEPTANCE → RESTAURANT_ACCEPTED → DRIVER_ASSIGNED → PREPARING → READY_FOR_PICKUP → OUT_FOR_DELIVERY → DELIVERED`. Failure terminals: `REJECTED`, `CANCELLED`, `FAILED`, `COMPENSATION_FAILED`.
