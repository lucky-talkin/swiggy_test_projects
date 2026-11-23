import json
import os
from urllib.error import HTTPError, URLError
from urllib.request import Request, urlopen

from temporalio import activity
from temporalio.exceptions import ApplicationError


def _request(base: str, path: str, method: str, payload: dict, *, ignore_404: bool = False) -> dict:
    request = Request(
        base.rstrip("/") + path,
        data=json.dumps(payload).encode(),
        headers={
            "Content-Type": "application/json",
            "X-Service-Token": os.getenv("INTERNAL_SERVICE_TOKEN", "dev-internal-token"),
        },
        method=method,
    )
    try:
        with urlopen(request, timeout=10) as response:
            raw = response.read()
            return json.loads(raw) if raw else {}
    except HTTPError as exc:
        if ignore_404 and exc.code == 404:
            return {"not_found": True}
        detail = exc.read().decode(errors="replace")
        non_retryable = 400 <= exc.code < 500 and exc.code not in (408, 409, 429)
        raise ApplicationError(
            f"{method} {path} returned HTTP {exc.code}: {detail}",
            type=f"HTTP_{exc.code}",
            non_retryable=non_retryable,
        ) from exc
    except (URLError, TimeoutError) as exc:
        raise ApplicationError(f"Downstream request failed: {exc}", type="DOWNSTREAM_UNAVAILABLE") from exc


@activity.defn
def create_restaurant_order(order: dict) -> dict:
    return _request(os.getenv("RESTAURANT_OPS_URL", "http://localhost:8084"), "/v1/restaurant-orders", "POST", order)


@activity.defn
def assign_driver(request: dict) -> dict:
    return _request(os.getenv("DRIVER_OPS_URL", "http://localhost:8085"), "/v1/assignments", "POST", request)


@activity.defn
def update_order_status(request: dict) -> dict:
    return _request(os.getenv("ORDER_API_URL", "http://localhost:8086"), f"/internal/orders/{request['order_id']}/status", "PATCH", request)


@activity.defn
def cancel_restaurant_order(request: dict) -> dict:
    return _request(os.getenv("RESTAURANT_OPS_URL", "http://localhost:8084"), f"/v1/restaurant-orders/{request['order_id']}/cancel", "POST", request, ignore_404=True)


@activity.defn
def release_driver_assignment(request: dict) -> dict:
    return _request(os.getenv("DRIVER_OPS_URL", "http://localhost:8085"), f"/v1/assignments/{request['order_id']}/release", "POST", request, ignore_404=True)
