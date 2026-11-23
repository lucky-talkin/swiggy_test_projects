# Search index contract

Search is a read-only query service. Catalog (or a dedicated indexing consumer) owns writes to the Elasticsearch index. Search never creates or mutates catalog documents.

## Index setup

- Default index: `food-search-v1`
- Override with `ELASTICSEARCH_INDEX`.
- Elasticsearch URL: `ELASTICSEARCH_URL`.
- Create the index once before using the API:

```bash
curl -X PUT 'http://localhost:9200/food-search-v1' \
  -H 'Content-Type: application/json' \
  --data-binary @infra/elasticsearch/food-search-v1-index.json
```

The explicit mapping is in [infra/elasticsearch/food-search-v1-index.json](../../../infra/elasticsearch/food-search-v1-index.json). It maps `location` as `geo_point`, and city/cuisines as text with normalized `.keyword` subfields for exact filters.

## Document shape

A restaurant or menu item is represented by a common search document:

```json
{
  "id": "restaurant-123",
  "entity_type": "restaurant",
  "restaurant_id": "restaurant-123",
  "name": "Dosa House",
  "description": "South Indian breakfast and tiffin",
  "cuisines": ["south indian", "vegetarian"],
  "category": "restaurant",
  "price_for_two": 350,
  "rating": 4.4,
  "delivery_time_minutes": 28,
  "is_open": true,
  "city": "bengaluru",
  "area": "indiranagar",
  "location": { "lat": 12.9784, "lon": 77.6408 },
  "popularity_score": 82.5
}
```

For menu items, set `entity_type` to `menu_item`, set `restaurant_id`, and populate `name`, `description`, `category`, and any applicable price fields.

## Query API

`GET /v1/search`

| Parameter | Description | Default / limit |
|---|---|---|
| `q` | Full-text query; optional for browse | 120 characters max |
| `type` | `restaurant` or `menu_item` | both |
| `city` | Exact city filter | optional |
| `cuisine` | Exact cuisine filter | optional |
| `min_rating` | Minimum rating | 0–5 |
| `lat`, `lon` | User coordinates; must be supplied together | optional |
| `radius_km` | Geo radius | 5 km default; 0.1–50 |
| `page` | 1-based page | 1 |
| `page_size` | Result count | 20; max 50 |

All results are filtered to currently open entities. Text relevance is ranked first, followed by `popularity_score`.

Example:

```bash
curl 'http://localhost:8083/v1/search?q=dosa&city=Bengaluru&type=restaurant&min_rating=4&page=1&page_size=10'
curl 'http://localhost:8083/v1/search?q=paneer&lat=12.9716&lon=77.5946&radius_km=5'
```

## Health

- `GET /healthz`: process liveness; does not require Elasticsearch.
- `GET /readyz`: readiness; returns 503 when Elasticsearch cannot be reached.
