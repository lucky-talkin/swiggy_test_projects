#!/bin/sh
set -eu

NAMESPACE=${DEFAULT_NAMESPACE:-default}
TEMPORAL_ADDRESS=${TEMPORAL_ADDRESS:-temporal:7233}
MAX_ATTEMPTS=${TEMPORAL_HEALTH_CHECK_MAX_ATTEMPTS:-30}
SLEEP_SECONDS=${TEMPORAL_HEALTH_CHECK_SLEEP_SECONDS:-5}

echo "Waiting for Temporal at $TEMPORAL_ADDRESS..."
attempt=1
while ! temporal operator cluster health --address "$TEMPORAL_ADDRESS" >/dev/null 2>&1; do
  if [ "$attempt" -ge "$MAX_ATTEMPTS" ]; then
    echo "Temporal did not become healthy after $MAX_ATTEMPTS attempts" >&2
    exit 1
  fi
  attempt=$((attempt + 1))
  sleep "$SLEEP_SECONDS"
done

if temporal operator namespace describe -n "$NAMESPACE" --address "$TEMPORAL_ADDRESS" >/dev/null 2>&1; then
  echo "Temporal namespace '$NAMESPACE' already exists."
  exit 0
fi

temporal operator namespace create -n "$NAMESPACE" --address "$TEMPORAL_ADDRESS"
echo "Created Temporal namespace '$NAMESPACE'."
