from datetime import timedelta
from temporalio import workflow

with workflow.unsafe.imports_passed_through():
    from activities.order import (
        assign_driver,
        compensate_order,
        complete_delivery,
        request_restaurant_acceptance,
        wait_for_restaurant_ready,
    )


@workflow.defn
class OrderWorkflow:
    """Durable order saga. Keep workflow code deterministic; all I/O is in activities."""

    @workflow.run
    async def run(self, order: dict) -> dict:
        order_id = order["order_id"]
        try:
            accepted = await workflow.execute_activity(
                request_restaurant_acceptance,
                order,
                start_to_close_timeout=timedelta(seconds=20),
                retry_policy=workflow.RetryPolicy(maximum_attempts=3),
            )
            if not accepted:
                raise RuntimeError("restaurant_rejected")

            driver = await workflow.execute_activity(
                assign_driver,
                order,
                start_to_close_timeout=timedelta(seconds=30),
                retry_policy=workflow.RetryPolicy(maximum_attempts=5),
            )
            await workflow.execute_activity(
                wait_for_restaurant_ready,
                {"order": order, "driver": driver},
                start_to_close_timeout=timedelta(minutes=45),
                retry_policy=workflow.RetryPolicy(maximum_attempts=3),
            )
            await workflow.execute_activity(
                complete_delivery,
                {"order": order, "driver": driver},
                start_to_close_timeout=timedelta(minutes=60),
            )
            return {"order_id": order_id, "status": "DELIVERED"}
        except Exception as exc:
            await workflow.execute_activity(
                compensate_order,
                {"order": order, "reason": str(exc)},
                start_to_close_timeout=timedelta(seconds=30),
                retry_policy=workflow.RetryPolicy(maximum_attempts=5),
            )
            return {"order_id": order_id, "status": "FAILED", "reason": str(exc)}
