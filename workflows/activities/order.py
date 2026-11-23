import json
import os
from urllib.request import Request, urlopen

def _post(base: str, path: str, payload: dict) -> dict:
    req = Request(base.rstrip("/") + path, data=json.dumps(payload).encode(), headers={"Content-Type": "application/json"}, method="POST")
    with urlopen(req, timeout=10) as response:
        return json.loads(response.read() or b"{}")

def request_restaurant_acceptance(order: dict) -> bool:
    result = _post(os.getenv("RESTAURANT_OPS_URL", "http://localhost:8084"), f"/v1/restaurant-orders/{order['order_id']}/accept", order)
    return result.get("accepted", True)

def assign_driver(order: dict) -> dict:
    return _post(os.getenv("DRIVER_OPS_URL", "http://localhost:8085"), "/v1/assignments", order)

def wait_for_restaurant_ready(context: dict) -> dict:
    # Replace with a durable Temporal signal from Restaurant Ops.
    return {"ready": True}

def complete_delivery(context: dict) -> dict:
    order_id = context["order"]["order_id"]
    return _post(os.getenv("DRIVER_OPS_URL", "http://localhost:8085"), f"/v1/assignments/{order_id}/delivered", context)

def compensate_order(context: dict) -> dict:
    # Implement idempotent driver release and restaurant cancellation.
    return {"compensated": True, "reason": context["reason"]}
