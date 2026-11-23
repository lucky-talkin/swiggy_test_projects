#!/usr/bin/env python3
"""Smoke-check the seeded documents through the public Search HTTP API."""
import json
import os
import sys
import urllib.parse
import urllib.request

BASE_URL = os.environ.get("SEARCH_API_URL", "http://localhost:8083").rstrip("/")

def search(**params):
    query = urllib.parse.urlencode(params)
    request = urllib.request.Request(f"{BASE_URL}/v1/search?{query}")
    with urllib.request.urlopen(request, timeout=10) as response:
        return json.load(response)

def main():
    restaurants = search(city="Bengaluru", type="restaurant", page_size=20)
    dishes = search(city="Bengaluru", type="menu_item", page_size=20)
    dosa = search(q="dosa", city="Bengaluru", type="menu_item", page_size=20)

    assert restaurants["total"] == 5, f"expected 5 restaurants, got {restaurants['total']}"
    assert dishes["total"] == 15, f"expected 15 menu items, got {dishes['total']}"
    results = dishes.get("items", [])
    masala_dosa = next((item for item in results if item.get("id") == "dish-1"), None)
    assert masala_dosa, "seeded Masala Dosa not found"
    assert masala_dosa.get("restaurant_id") == "restaurant-1", "Masala Dosa restaurant association is wrong"
    assert masala_dosa.get("price") == 90, f"expected Masala Dosa price 90, got {masala_dosa.get('price')}"
    assert any(item.get("id") == "dish-1" for item in dosa.get("items", [])), "full-text dosa query did not return Masala Dosa"

    print("Search seed verification passed: 5 restaurants, 15 menu items, dosa query and checkout fields.")
    return 0

if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (AssertionError, KeyError, OSError, ValueError) as error:
        print(f"Search seed verification failed: {error}", file=sys.stderr)
        raise SystemExit(1)
