import asyncio
import os
from temporalio.client import Client
from temporalio.worker import Worker
from workflows.order import OrderWorkflow
from activities.order import assign_driver, compensate_order, complete_delivery, request_restaurant_acceptance, wait_for_restaurant_ready

async def main():
    client = await Client.connect(os.getenv("TEMPORAL_ADDRESS", "localhost:7233"), namespace=os.getenv("TEMPORAL_NAMESPACE", "default"))
    worker = Worker(client, task_queue="food-delivery-orders", workflows=[OrderWorkflow], activities=[request_restaurant_acceptance, assign_driver, wait_for_restaurant_ready, complete_delivery, compensate_order])
    await worker.run()

if __name__ == "__main__":
    asyncio.run(main())
