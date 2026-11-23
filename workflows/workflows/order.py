from datetime import timedelta
from temporalio import workflow
from temporalio.common import RetryPolicy


class SagaFailure(Exception):
    def __init__(self, code: str, message: str):
        super().__init__(message)
        self.code = code


@workflow.defn
class OrderWorkflow:
    """Coordinates a food-order saga. Network and database I/O stays in activities."""

    def __init__(self) -> None:
        self._restaurant_decision: str | None = None
        self._restaurant_ready = False
        self._driver_picked_up = False
        self._delivery_completed = False
        self._cancel_reason: str | None = None

    @workflow.signal
    def restaurant_decision(self, event: dict) -> None:
        if self._restaurant_decision is None:
            self._restaurant_decision = str(event.get("decision", "")).upper()

    @workflow.signal
    def restaurant_ready(self, event: dict) -> None:
        self._restaurant_ready = True

    @workflow.signal
    def driver_picked_up(self, event: dict) -> None:
        self._driver_picked_up = True

    @workflow.signal
    def delivery_completed(self, event: dict) -> None:
        self._delivery_completed = True

    @workflow.signal
    def cancel_order(self, event: dict) -> None:
        # A cancellation signal arriving after the courier confirms delivery
        # must not turn a successfully delivered order into a cancelled one.
        if self._delivery_completed:
            return
        if self._cancel_reason is None:
            self._cancel_reason = str(event.get("reason") or "cancelled_by_customer")

    async def _wait_or_cancel(self, predicate, timeout: timedelta, timeout_code: str) -> None:
        try:
            await workflow.wait_condition(
                lambda: self._cancel_reason is not None or predicate(),
                timeout=timeout,
            )
        except TimeoutError as exc:
            raise SagaFailure(timeout_code, f"Timed out waiting for {timeout_code}") from exc
        if self._cancel_reason is not None:
            raise SagaFailure("CANCELLED", self._cancel_reason)

    @workflow.run
    async def run(self, order: dict) -> dict:
        order_id = order["order_id"]
        # Register compensations before commands: a timeout may occur after a
        # downstream commit but before the activity receives its response.
        compensations: list[tuple[str, dict]] = []
        normal_retry = RetryPolicy(
            initial_interval=timedelta(seconds=1),
            backoff_coefficient=2.0,
            maximum_interval=timedelta(seconds=15),
            maximum_attempts=5,
        )
        compensation_retry = RetryPolicy(
            initial_interval=timedelta(seconds=2),
            backoff_coefficient=2.0,
            maximum_interval=timedelta(minutes=1),
            maximum_attempts=0,
        )

        async def set_status(status: str, reason: str = "") -> None:
            await workflow.execute_activity(
                "update_order_status",
                {"order_id": order_id, "status": status, "reason": reason},
                start_to_close_timeout=timedelta(seconds=10),
                retry_policy=normal_retry,
            )

        failure_code = ""
        failure_message = ""
        terminal_status = "FAILED"

        try:
            await set_status("AWAITING_RESTAURANT_ACCEPTANCE")

            compensations.append(("cancel_restaurant_order", {"order_id": order_id}))
            await workflow.execute_activity(
                "create_restaurant_order",
                order,
                start_to_close_timeout=timedelta(seconds=15),
                retry_policy=normal_retry,
            )
            await self._wait_or_cancel(
                lambda: self._restaurant_decision is not None,
                timedelta(minutes=3),
                "RESTAURANT_ACCEPTANCE_TIMEOUT",
            )
            if self._restaurant_decision != "ACCEPTED":
                raise SagaFailure("RESTAURANT_REJECTED", "Restaurant rejected the order")
            await set_status("RESTAURANT_ACCEPTED")

            compensations.append(("release_driver_assignment", {"order_id": order_id}))
            driver = await workflow.execute_activity(
                "assign_driver",
                {"order_id": order_id, "restaurant_id": order["restaurant_id"]},
                start_to_close_timeout=timedelta(seconds=20),
                retry_policy=RetryPolicy(
                    initial_interval=timedelta(seconds=2),
                    backoff_coefficient=2.0,
                    maximum_interval=timedelta(seconds=20),
                    maximum_attempts=6,
                ),
            )
            await set_status("DRIVER_ASSIGNED")
            await set_status("PREPARING")

            await self._wait_or_cancel(
                lambda: self._restaurant_ready,
                timedelta(minutes=45),
                "RESTAURANT_PREPARATION_TIMEOUT",
            )
            await set_status("READY_FOR_PICKUP")

            await self._wait_or_cancel(
                lambda: self._driver_picked_up,
                timedelta(minutes=15),
                "DRIVER_PICKUP_TIMEOUT",
            )
            await set_status("OUT_FOR_DELIVERY")

            await self._wait_or_cancel(
                lambda: self._delivery_completed,
                timedelta(minutes=90),
                "DELIVERY_TIMEOUT",
            )
            await set_status("DELIVERED")
            await workflow.wait_condition(workflow.all_handlers_finished)
            return {"order_id": order_id, "status": "DELIVERED", "driver_id": driver.get("driver_id", "")}

        except SagaFailure as exc:
            failure_code, failure_message = exc.code, str(exc)
            terminal_status = (
                "CANCELLED" if exc.code == "CANCELLED"
                else "REJECTED" if exc.code == "RESTAURANT_REJECTED"
                else "FAILED"
            )
        except Exception as exc:
            failure_code, failure_message = "SAGA_STEP_FAILED", str(exc)
            terminal_status = "FAILED"

        compensation_errors: list[str] = []
        for activity_name, payload in reversed(compensations):
            try:
                await workflow.execute_activity(
                    activity_name,
                    payload,
                    start_to_close_timeout=timedelta(seconds=20),
                    retry_policy=compensation_retry,
                )
            except Exception as exc:
                compensation_errors.append(f"{activity_name}: {exc}")

        if compensation_errors:
            terminal_status = "COMPENSATION_FAILED"
            failure_message = f"{failure_message}; compensation errors: {'; '.join(compensation_errors)}"

        await set_status(terminal_status, f"{failure_code}: {failure_message}")
        await workflow.wait_condition(workflow.all_handlers_finished)
        return {
            "order_id": order_id,
            "status": terminal_status,
            "failure_code": failure_code,
            "reason": failure_message,
            "compensations_attempted": len(compensations),
            "compensation_errors": compensation_errors,
        }
