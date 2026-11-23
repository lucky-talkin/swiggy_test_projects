# Food delivery web app

React + TypeScript + Vite frontend for the food-delivery microservices repository.

## Run

From this directory:

```bash
npm install
npm run dev
```

Open http://localhost:5173. Start the backend from the repository root with `docker compose up --build`.

Vite proxies browser requests to the local services:
- `/search-api/*` → Search API at `localhost:8083`
- `/order-api/*` → Order API at `localhost:8086`
- `/catalog-api/*` → Catalog API at `localhost:8082`
- `/auth-api/*` → User/Auth API at `localhost:8081`

Override targets with environment variables; see `.env.example`.

## Implemented integration

- Search: `GET /v1/search?q=&city=&type=&page=&page_size=`
- Create order: `POST /v1/orders` with an `Idempotency-Key` header and the backend's `customer_id`, `restaurant_id`, `items`, `delivery_address`, `total_amount_minor`, and `currency` contract.
- Redux Toolkit stores the cart; RTK Query handles API requests, loading/error states and caching.

The search API index document includes restaurant and menu-item metadata. The UI expects a result list in `results` or `items`, with item `id`, `restaurant_id`, `name`, and `price` for adding a menu item to the cart. Search results that don't contain the required checkout identifiers/prices remain visible but cannot be added.

## Backend limitations

The current repository still scaffolds User/Auth registration and login and Catalog restaurant read/write endpoints as not implemented. This frontend therefore integrates the implemented Search and Order APIs; it does not pretend login, catalog menu retrieval, payment, or live courier tracking work. The Order API requires an `Idempotency-Key`; the frontend generates a fresh UUID per checkout. Order amounts are sent in minor currency units (paise for INR).

Search results are only as complete as the Elasticsearch `food-search-v1` index. Populate it through the catalog/indexing path before expecting restaurant results.