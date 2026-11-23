# Food Delivery Platform

A microservices-based food delivery backend inspired by Swiggy. The repository is organized as independently deployable Go services with Python Temporal orchestration.

## Services

| Service | Responsibility | Persistence |
|---|---|---|
| user-auth | Registration, login, JWT, user profiles | PostgreSQL |
| catalog | Restaurant/menu management and catalog reads | Elasticsearch |
| search | Full-text restaurant and dish search | Elasticsearch |
| restaurant-ops | Restaurant acceptance, preparation and handoff state | PostgreSQL |
| driver-ops | Driver availability, assignment and delivery state | PostgreSQL |
| order-api | Order placement/read API and workflow start | PostgreSQL (order record) |
| temporal-worker | Order lifecycle orchestration, retries, timers and compensation | Temporal (Python SDK) |

Kafka is used for domain-event fan-out where asynchronous projections are needed. Redis is used for short-lived caching/coordination. Services own their databases; cross-service writes are orchestrated through Temporal rather than distributed SQL transactions.

## Local development

Requirements: Docker Compose, Go 1.23+, Python 3.12+.

```bash
cp .env.example .env
docker compose up --build
```

Infrastructure endpoints:
- PostgreSQL: localhost:5432
- Elasticsearch: http://localhost:9200
- Redis: localhost:6379
- Kafka: localhost:9092
- Temporal UI: http://localhost:8088
- Temporal gRPC: localhost:7233

Service HTTP ports: user-auth 8081, catalog 8082, search 8083, restaurant-ops 8084, driver-ops 8085, order-api 8086.

## Order lifecycle

1. Order API validates the request, persists an order as `PENDING`, and starts a Temporal workflow using the order ID as the workflow ID (idempotent start).
2. Workflow requests restaurant acceptance. Restaurant Ops owns acceptance and preparation state.
3. After acceptance, workflow requests driver assignment. Driver Ops owns availability and assignment.
4. Workflow waits for preparation and pickup/delivery signals, applying bounded retries and timeouts.
5. Success marks the order delivered. Rejection, timeout or unrecoverable failure triggers compensating activities (release driver, cancel restaurant ticket, refund placeholder) and a terminal state.

Temporal activities call service APIs; the workflow contains orchestration only. Activities must be idempotent because Temporal can retry them.

## Repository layout

See [docs/architecture.md](docs/architecture.md) and [docs/api-contracts.md](docs/api-contracts.md). Each Go service has its own module and Dockerfile. The Python worker is in `workflows/`.

## Development status

This is an executable architecture scaffold: health endpoints, service boundaries, infrastructure and workflow contracts are established. Business persistence, authentication hardening, payment integration, geospatial driver matching and production observability are intentionally incremental follow-up work.
