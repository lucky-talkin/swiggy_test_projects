#!/usr/bin/env python3
"""Seed deterministic Bengaluru restaurant and menu documents into Elasticsearch.

Safe to run repeatedly: documents use stable IDs and are indexed with the
Bulk API's `index` operation (upsert). Existing unrelated documents remain.
"""
from __future__ import annotations

import json
import os
import sys
import urllib.error
import urllib.request
from pathlib import Path

BASE_URL = os.environ.get("ELASTICSEARCH_URL", "http://localhost:9200").rstrip("/")
INDEX = os.environ.get("ELASTICSEARCH_INDEX", "food-search-v1")
MAPPING_FILE = Path(__file__).with_name("food-search-v1-index.json")

RESTAURANTS = [
    {
        "id": "restaurant-1", "name": "Dosa House", "description": "Crispy South Indian dosas, idlis and traditional filter coffee",
        "cuisines": ["south indian", "vegetarian"], "category": "restaurant", "price_for_two": 350,
        "rating": 4.6, "delivery_time_minutes": 25, "is_open": True, "city": "bengaluru",
        "area": "indiranagar", "location": {"lat": 12.9784, "lon": 77.6408}, "popularity_score": 98.0,
    },
    {
        "id": "restaurant-2", "name": "Andhra Spice Kitchen", "description": "Authentic Andhra meals, spicy biryani and homestyle curries",
        "cuisines": ["andhra", "south indian"], "category": "restaurant", "price_for_two": 500,
        "rating": 4.4, "delivery_time_minutes": 32, "is_open": True, "city": "bengaluru",
        "area": "koramangala", "location": {"lat": 12.9352, "lon": 77.6245}, "popularity_score": 93.0,
    },
    {
        "id": "restaurant-3", "name": "Punjabi Tadka", "description": "North Indian comfort food, paneer specials and buttery naan",
        "cuisines": ["north indian", "punjabi"], "category": "restaurant", "price_for_two": 600,
        "rating": 4.3, "delivery_time_minutes": 35, "is_open": True, "city": "bengaluru",
        "area": "hsr layout", "location": {"lat": 12.9116, "lon": 77.6389}, "popularity_score": 89.0,
    },
    {
        "id": "restaurant-4", "name": "Bangalore Biryani Co", "description": "Dum biryani, kebabs and classic local rice dishes",
        "cuisines": ["biryani", "north indian"], "category": "restaurant", "price_for_two": 450,
        "rating": 4.5, "delivery_time_minutes": 30, "is_open": True, "city": "bengaluru",
        "area": "jayanagar", "location": {"lat": 12.9250, "lon": 77.5838}, "popularity_score": 96.0,
    },
    {
        "id": "restaurant-5", "name": "Green Bowl Cafe", "description": "Fresh salads, wholesome bowls and vegetarian cafe favourites",
        "cuisines": ["healthy food", "continental", "vegetarian"], "category": "restaurant", "price_for_two": 400,
        "rating": 4.2, "delivery_time_minutes": 22, "is_open": True, "city": "bengaluru",
        "area": "whitefield", "location": {"lat": 12.9698, "lon": 77.7500}, "popularity_score": 84.0,
    },
]

MENU = [
    ("dish-1", "restaurant-1", "Masala Dosa", "Crispy dosa filled with spiced potato masala", "breakfast", 90, ["south indian", "vegetarian"]),
    ("dish-2", "restaurant-1", "Plain Dosa", "Golden, crisp dosa served with chutney and sambar", "breakfast", 65, ["south indian", "vegetarian"]),
    ("dish-3", "restaurant-1", "Idli Vada", "Steamed idlis and a crisp medu vada", "breakfast", 75, ["south indian", "vegetarian"]),
    ("dish-4", "restaurant-2", "Andhra Chicken Biryani", "Spicy dum-cooked chicken biryani", "biryani", 240, ["andhra", "biryani"]),
    ("dish-5", "restaurant-2", "Gongura Paneer", "Paneer cooked with tangy gongura leaves", "main course", 190, ["andhra", "vegetarian"]),
    ("dish-6", "restaurant-2", "Andhra Veg Meals", "Traditional rice, dal, vegetable curries and pickle", "thali", 160, ["andhra", "vegetarian"]),
    ("dish-7", "restaurant-3", "Paneer Butter Masala", "Paneer cubes in a rich tomato and butter gravy", "main course", 210, ["north indian", "punjabi", "vegetarian"]),
    ("dish-8", "restaurant-3", "Dal Makhani", "Slow-cooked black lentils finished with cream", "main course", 180, ["north indian", "punjabi", "vegetarian"]),
    ("dish-9", "restaurant-3", "Butter Naan", "Soft tandoor naan brushed with butter", "bread", 45, ["north indian", "punjabi", "vegetarian"]),
    ("dish-10", "restaurant-4", "Bangalore Chicken Biryani", "Aromatic local-style chicken biryani", "biryani", 220, ["biryani", "south indian"]),
    ("dish-11", "restaurant-4", "Mutton Dum Biryani", "Slow-cooked mutton dum biryani", "biryani", 310, ["biryani"]),
    ("dish-12", "restaurant-4", "Chicken Seekh Kebab", "Grilled chicken kebabs with mint chutney", "starter", 170, ["north indian"]),
    ("dish-13", "restaurant-5", "Paneer Protein Bowl", "Grilled paneer, brown rice and fresh vegetables", "healthy bowls", 210, ["healthy food", "vegetarian"]),
    ("dish-14", "restaurant-5", "Avocado Salad", "Fresh avocado, greens, tomato and citrus dressing", "salad", 230, ["healthy food", "continental", "vegetarian"]),
    ("dish-15", "restaurant-5", "Quinoa Veg Bowl", "Quinoa with roasted seasonal vegetables", "healthy bowls", 195, ["healthy food", "vegetarian"]),
]

def request(method: str, path: str, body: bytes | None = None, content_type: str = "application/json"):
    req = urllib.request.Request(
        f"{BASE_URL}/{path.lstrip('/')}", data=body, method=method,
        headers={"Content-Type": content_type},
    )
    with urllib.request.urlopen(req, timeout=15) as response:
        return response.status, response.read()

def main() -> int:
    try:
        try:
            request("HEAD", INDEX)
            print(f"Index {INDEX!r} already exists; keeping its mapping.")
        except urllib.error.HTTPError as error:
            if error.code != 404:
                raise
            mapping = MAPPING_FILE.read_bytes()
            request("PUT", INDEX, mapping)
            print(f"Created index {INDEX!r} from {MAPPING_FILE.name}.")

        # Older local indices may predate the menu-item price field.
        # Add it idempotently so strict mappings accept seeded menu documents.
        request("PUT", f"{INDEX}/_mapping", json.dumps({"properties": {"price": {"type": "integer"}}}).encode())

        restaurants = []
        for item in RESTAURANTS:
            restaurants.append({**item, "entity_type": "restaurant", "restaurant_id": item["id"]})

        restaurant_by_id = {item["id"]: item for item in RESTAURANTS}
        dishes = []
        for dish_id, restaurant_id, name, description, category, price, cuisines in MENU:
            restaurant = restaurant_by_id[restaurant_id]
            dishes.append({
                "id": dish_id, "entity_type": "menu_item", "restaurant_id": restaurant_id,
                "name": name, "description": description, "cuisines": cuisines,
                "category": category, "price": price, "rating": restaurant["rating"],
                "delivery_time_minutes": restaurant["delivery_time_minutes"], "is_open": True,
                "city": restaurant["city"], "area": restaurant["area"],
                "location": restaurant["location"], "popularity_score": restaurant["popularity_score"],
            })

        lines = []
        for document in restaurants + dishes:
            lines.append(json.dumps({"index": {"_index": INDEX, "_id": document["id"]}}, separators=(",", ":")))
            lines.append(json.dumps(document, separators=(",", ":")))
        payload = ("\n".join(lines) + "\n").encode()
        _, raw = request("POST", "_bulk", payload, "application/x-ndjson")
        result = json.loads(raw)
        failures = [item for item in result.get("items", []) if item.get("index", {}).get("error")]
        if result.get("errors") or failures:
            print(json.dumps(failures, indent=2), file=sys.stderr)
            return 1
        request("POST", f"{INDEX}/_refresh")
        print(f"Seeded {len(restaurants)} restaurants and {len(dishes)} menu items into {INDEX!r}.")
        print("Try: curl 'http://localhost:8083/v1/search?city=Bengaluru&type=restaurant'")
        print("Try: curl 'http://localhost:8083/v1/search?q=dosa&city=Bengaluru&type=menu_item'")
        return 0
    except (OSError, urllib.error.URLError, json.JSONDecodeError) as error:
        print(f"Seeding failed: {error}", file=sys.stderr)
        return 1

if __name__ == "__main__":
    raise SystemExit(main())
