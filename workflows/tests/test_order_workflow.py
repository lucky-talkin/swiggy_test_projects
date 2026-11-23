import asyncio
import uuid

import pytest
from temporalio import activity
from temporalio.testing import WorkflowEnvironment
from temporalio.worker import Worker

from workflows.workflows.order import OrderWorkflow


def make_activity_set():
    calls: list[tuple[str, dict]] = []
    restaurant_created = asyncio.Event()
    driver_assigned = asyncio.Event()

    @activity.defn(name="create_restaurant_order")
    async def create_restaurant_order(order: dict) -> dict:
        calls.append(("restaurant-create", order))
        restaurant_created.set()
        return {"created": True}

    @activity.defn(name="assign_driver")
    async def assign_driver(request: dict) -> dict:
        calls.append(("driver-assign", request))
        driver_assigned.set()
        return {"driver_id": "driver-test-1"}

    @activity.defn(name="update_order_status")
    async def update_order_status(request: dict) -> dict:
        calls.append(("status", request))
        return {"updated": True}

    @activity.defn(name="cancel_restaurant_order")
    async def cancel_restaurant_order(request: dict) -> dict:
        calls.append(("restaurant-cancel", request))
        return {"cancelled": True}

    @activity.defn(name="release_driver_assignment")
    async def release_driver_assignment(request: dict) -> dict:
        calls.append(("driver-release", request))
        return {"released": True}

    return calls, restaurant_created, driver_assigned, [
        create_restaurant_order,
        assign_driver,
        update_order_status,
        cancel_restaurant_order,
        release_driver_assignment,
    ]


async def start_order(env, activities, order_id):
    queue = "order-saga-" + str(uuid.uuid4())
    worker = Worker(env.client, task_queue=queue, workflows=[OrderWorkflow], activities=activities)
    return queue, worker


@pytest.mark.asyncio
async def test_happy_path_delivers_without_compensation():
    calls, restaurant_created, driver_assigned, activities = make_activity_set()
    async with await WorkflowEnvironment.start_time_skipping() as env:
        queue = "order-saga-" + str(uuid.uuid4())
        async with Worker(env.client, task_queue=queue, workflows=[OrderWorkflow], activities=activities):
            handle = await env.client.start_workflow(
                OrderWorkflow.run,
                {"order_id": "success-1", "restaurant_id": "restaurant-1"},
                id="success-" + str(uuid.uuid4()), task_queue=queue,
            )
            await asyncio.wait_for(restaurant_created.wait(), timeout=10)
            await handle.signal(OrderWorkflow.restaurant_decision, {"decision": "ACCEPTED"})
            await asyncio.wait_for(driver_assigned.wait(), timeout=10)
            await handle.signal(OrderWorkflow.restaurant_ready, {})
            await handle.signal(OrderWorkflow.driver_picked_up, {})
            await handle.signal(OrderWorkflow.delivery_completed, {})
            result = await handle.result()

    assert result["status"] == "DELIVERED"
    assert any(name == "status" and payload["status"] == "DELIVERED" for name, payload in calls)
    assert not any(name.endswith("cancel") or name.endswith("release") for name, _ in calls)


@pytest.mark.asyncio
async def test_rejected_order_compensates_restaurant_only():
    calls, restaurant_created, _, activities = make_activity_set()
    async with await WorkflowEnvironment.start_time_skipping() as env:
        queue = "order-saga-" + str(uuid.uuid4())
        async with Worker(env.client, task_queue=queue, workflows=[OrderWorkflow], activities=activities):
            handle = await env.client.start_workflow(
                OrderWorkflow.run,
                {"order_id": "reject-1", "restaurant_id": "restaurant-1"},
                id="reject-" + str(uuid.uuid4()), task_queue=queue,
            )
            await asyncio.wait_for(restaurant_created.wait(), timeout=10)
            await handle.signal(OrderWorkflow.restaurant_decision, {"decision": "REJECTED"})
            result = await handle.result()

    names = [name for name, _ in calls]
    assert result["status"] == "REJECTED"
    assert "restaurant-cancel" in names
    assert "driver-assign" not in names
    assert "driver-release" not in names


@pytest.mark.asyncio
async def test_customer_cancellation_releases_driver_then_restaurant():
    calls, restaurant_created, driver_assigned, activities = make_activity_set()
    async with await WorkflowEnvironment.start_time_skipping() as env:
        queue = "order-saga-" + str(uuid.uuid4())
        async with Worker(env.client, task_queue=queue, workflows=[OrderWorkflow], activities=activities):
            handle = await env.client.start_workflow(
                OrderWorkflow.run,
                {"order_id": "cancel-1", "restaurant_id": "restaurant-1"},
                id="cancel-" + str(uuid.uuid4()), task_queue=queue,
            )
            await asyncio.wait_for(restaurant_created.wait(), timeout=10)
            await handle.signal(OrderWorkflow.restaurant_decision, {"decision": "ACCEPTED"})
            await asyncio.wait_for(driver_assigned.wait(), timeout=10)
            await handle.signal(OrderWorkflow.cancel_order, {"reason": "customer_changed_mind"})
            result = await handle.result()

    names = [name for name, _ in calls]
    assert result["status"] == "CANCELLED"
    assert names.index("driver-release") < names.index("restaurant-cancel")


@pytest.mark.asyncio
async def test_preparation_timeout_runs_compensations_in_reverse_order():
    calls, restaurant_created, driver_assigned, activities = make_activity_set()
    async with await WorkflowEnvironment.start_time_skipping() as env:
        queue = "order-saga-" + str(uuid.uuid4())
        async with Worker(env.client, task_queue=queue, workflows=[OrderWorkflow], activities=activities):
            handle = await env.client.start_workflow(
                OrderWorkflow.run,
                {"order_id": "timeout-1", "restaurant_id": "restaurant-1"},
                id="timeout-" + str(uuid.uuid4()), task_queue=queue,
            )
            await asyncio.wait_for(restaurant_created.wait(), timeout=10)
            await handle.signal(OrderWorkflow.restaurant_decision, {"decision": "ACCEPTED"})
            await asyncio.wait_for(driver_assigned.wait(), timeout=10)
            result = await handle.result()

    names = [name for name, _ in calls]
    assert result["status"] == "FAILED"
    assert result["failure_code"] == "RESTAURANT_PREPARATION_TIMEOUT"
    assert names.index("driver-release") < names.index("restaurant-cancel")
