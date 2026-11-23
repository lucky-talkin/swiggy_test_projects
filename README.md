# Food Delivery Platform

A microservices-based food-delivery backend built for distributed-systems practice. Go services own their data and REST APIs; a Python Temporal worker orchestrates the long-running order saga.

## Services and data ownership

| Service | Responsibility | Store |
|---|---|---|
| user-auth | Registration, login, identity and profiles | PostgreSQL |
| catalog | Restaurant/menu catalog | Elasticsearch |
| search | Full-text restaurant and menu search | Elasticsearch |
| restaurant-ops | Restaurant order acceptance, preparation and readiness | PostgreSQL |
| driver-ops | Driver availability, assignment, pickup and delivery | PostgreSQL |
| order-api | Customer orders, idempotency, status history and workflow start | PostgreSQL |
| temporal-worker | Durable order orchestration and compensation | Temporal (Python SDK) |

Redis and Kafka are included as supporting infrastructure for future caching, geo lookup and asynchronous domain-event fan-out. The current order saga uses transactional outboxes for reliable handoff to Temporal.

## Run locally

Requirements: Docker Compose, internet access for image/dependency downloads.

```bash
cp .env.example .env
docker compose up --build
```

Infrastructure:
- PostgreSQL: localhost:5432
- Elasticsearch: http://localhost:9200
- Redis: localhost:6379
- Kafka: localhost:9092
- Temporal gRPC: localhost:7233
- Temporal UI: http://localhost:8088

APIs: user-auth 8081, catalog 8082, search 8083, restaurant-ops 8084, driver-ops 8085, order-api 8086.

Temporal admin-tools initializes the separate Temporal persistence and visibility schemas in PostgreSQL and creates the `default` namespace before the order API and worker start. Go services apply their own migrations at startup. The local driver service seeds three demo drivers.

## Search API

Search is a read-only query service over the Elasticsearch `food-search-v1` index. Catalog or a dedicated indexing consumer owns writes; Search does not mutate catalog documents.

Create the index mapping once, then query the service:

```bash
curl -X PUT 'http://localhost:9200/food-search-v1' \
  -H 'Content-Type: application/json' \
  --data-binary @infra/elasticsearch/food-search-v1-index.json

curl 'http://localhost:8083/v1/search?q=dosa&city=Bengaluru&type=restaurant&min_rating=4&page=1&page_size=10'
curl 'http://localhost:8083/v1/search?q=paneer&lat=12.9716&lon=77.5946&radius_km=5'
curl http://localhost:8083/healthz
curl http://localhost:8083/readyz
```

Supported filters include entity type, city, cuisine, minimum rating and geo radius. Pagination is bounded to 50 results per page. See [services/search/docs/index-contract.md](services/search/docs/index-contract.md) for the Elasticsearch document mapping and API contract. The index must be populated separately; the Search service intentionally does not create fake catalog data.

## Order lifecycle — Temporal Saga

The order is a distributed business process, not a database transaction across services.

1. Order API validates the request, requires an `Idempotency-Key`, writes the order and workflow-start outbox row in one PostgreSQL transaction, then returns `202 Accepted`.
2. Outbox dispatcher starts `OrderWorkflow` with workflow ID `order-{order_id}`.
3. Temporal creates the restaurant ticket, waits for the restaurant decision, and compensates the ticket if the restaurant rejects or times out.
4. After acceptance, Temporal assigns a driver. Driver Ops atomically claims an available driver and records the assignment.
5. Workflow waits for restaurant readiness, driver pickup and delivery completion using durable Temporal signals and timers.
6. On cancellation or failure, compensations run in reverse order: release the driver assignment, then cancel the restaurant ticket.
7. A driver who has already picked up the order is marked `DELIVERY_EXCEPTION` rather than incorrectly returned to the available pool.

See [docs/order-saga.md](docs/order-saga.md) for the sequence, API walkthrough, retry policy and failure semantics. See [docs/architecture.md](docs/architecture.md) for service boundaries.

## Try the happy path

Create an order (amount is in minor units: 45000 paise = ₹450):

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

Copy the returned order ID, then drive the restaurant and courier events:

```bash
export ORDER_ID=<returned-order-id>
export TOKEN=dev-internal-token

curl -X POST http://localhost:8084/v1/restaurant-orders/$ORDER_ID/accept -H "X-Service-Token: $TOKEN"
curl -X POST http://localhost:8084/v1/restaurant-orders/$ORDER_ID/ready -H "X-Service-Token: $TOKEN"

# Wait for the driver assignment to appear in the canonical order state.
until curl -s http://localhost:8086/v1/orders/$ORDER_ID | grep -Eq '"status":"(DRIVER_ASSIGNED|PREPARING|READY_FOR_PICKUP)"'; do sleep 1; done

curl -X POST http://localhost:8085/v1/assignments/$ORDER_ID/picked-up -H "X-Service-Token: $TOKEN"
curl -X POST http://localhost:8085/v1/assignments/$ORDER_ID/delivered -H "X-Service-Token: $TOKEN"

curl http://localhost:8086/v1/orders/$ORDER_ID
```

Try rejection or cancellation to observe compensation:

```bash
curl -X POST http://localhost:8084/v1/restaurant-orders/$ORDER_ID/reject -H "X-Service-Token: $TOKEN"
# or
curl -X POST http://localhost:8086/v1/orders/$ORDER_ID/cancel \
  -H 'Content-Type: application/json' -d '{"reason":"customer_changed_mind"}'
```

Open Temporal UI at http://localhost:8088 and inspect the `order-{order_id}` execution and its event history.

## Tests

Run Temporal workflow tests locally:

```bash
python -m pip install -r workflows/requirements.txt
python -m pytest -q workflows/tests
```

The tests exercise successful delivery, restaurant rejection, customer cancellation and preparation timeout. The Temporal time-skipping test server is downloaded by the SDK as part of the test environment.

Go services can be tested independently from their service directories with `go test ./...`.

## Production hardening backlog

This is an engineering portfolio project scaffold with a functioning saga path, not a production-ready food marketplace. Next steps include user authentication/authorization on public APIs, service-specific PostgreSQL databases and roles, payment authorization/capture/refund activities, transactional inbox reconciliation and alerting, real driver geospatial matching, catalog indexing, Kafka outbox publishing, observability, and load/failure testing.

Payment is intentionally not simulated. Do not claim that a customer has been charged or refunded until a payment provider and its idempotent compensation are integrated.
