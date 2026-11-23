# Initial API contracts

All JSON APIs use `Content-Type: application/json`. Add request validation, pagination, correlation IDs and consistent error envelopes before production.

## User/Auth — :8081
- `POST /v1/auth/register` — create account.
- `POST /v1/auth/login` — issue access token.
- `GET /healthz`.

## Catalog — :8082
- `PUT /v1/restaurants/{id}` — upsert restaurant document into Elasticsearch.
- `GET /v1/restaurants/{id}` — retrieve restaurant.
- `GET /healthz`.

## Search — :8083
- `GET /v1/search?q=...` — full-text restaurant/menu search.
- `GET /healthz`.

## Restaurant Ops — :8084
- `POST /v1/restaurant-orders/{order_id}/accept`
- `POST /v1/restaurant-orders/{order_id}/reject`
- `POST /v1/restaurant-orders/{order_id}/ready`
- `GET /healthz`.

## Driver Ops — :8085
- `POST /v1/drivers/{driver_id}/availability`
- `POST /v1/assignments` — idempotently assign a driver to an order.
- `POST /v1/assignments/{order_id}/picked-up`
- `POST /v1/assignments/{order_id}/delivered`
- `GET /healthz`.

## Order API — :8086
- `POST /v1/orders` — accepts customer, restaurant, items and idempotency key; starts Temporal workflow.
- `GET /v1/orders/{id}` — read order status.
- `GET /healthz`.

Workflow contract: `OrderWorkflow(order_id, customer_id, restaurant_id)`. Activities: `request_restaurant_acceptance`, `assign_driver`, `wait_for_restaurant_ready`, `complete_delivery`, `compensate_order`. Signals: `restaurant_ready`, `delivery_completed`, `cancel_order`.
