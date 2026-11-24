import asyncio
from concurrent.futures import ThreadPoolExecutor
import logging
import os

from temporalio.client import Client
from temporalio.worker import Worker

from workflows.activities.order import (
    assign_driver,
    cancel_restaurant_order,
    create_restaurant_order,
    release_driver_assignment,
    update_order_status,
)
from workflows.workflows.order import OrderWorkflow

logging.basicConfig(level=os.getenv("LOG_LEVEL", "INFO"))
logger = logging.getLogger("food-delivery-temporal-worker")


async def main() -> None:
    address = os.getenv("TEMPORAL_ADDRESS", "localhost:7233")
    namespace = os.getenv("TEMPORAL_NAMESPACE", "default")
    while True:
        try:
            client = await Client.connect(address, namespace=namespace)
            break
        except Exception:
            logger.exception("Temporal is not ready at %s; retrying in 3 seconds", address)
            await asyncio.sleep(3)

    logger.info("Connected to Temporal at %s (namespace=%s)", address, namespace)
    # These activities perform blocking HTTP calls with urllib. Temporal's Python
    # worker requires an executor for synchronous activity functions.
    max_workers = max(1, int(os.getenv("ACTIVITY_EXECUTOR_MAX_WORKERS", "10")))
    activity_executor = ThreadPoolExecutor(
        max_workers=max_workers,
        thread_name_prefix="temporal-activity",
    )
    worker = Worker(
        client,
        task_queue=os.getenv("TEMPORAL_TASK_QUEUE", "food-delivery-orders"),
        workflows=[OrderWorkflow],
        activities=[
            create_restaurant_order,
            assign_driver,
            update_order_status,
            cancel_restaurant_order,
            release_driver_assignment,
        ],
        activity_executor=activity_executor,
    )
    try:
        await worker.run()
    finally:
        activity_executor.shutdown(wait=True, cancel_futures=True)


if __name__ == "__main__":
    asyncio.run(main())
