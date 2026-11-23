#!/bin/sh
set -eu

DB_PORT=${DB_PORT:-5432}
DBNAME=${DBNAME:-temporal}
VISIBILITY_DBNAME=${VISIBILITY_DBNAME:-temporal_visibility}

: "${POSTGRES_SEEDS:?POSTGRES_SEEDS is required}"
: "${POSTGRES_USER:?POSTGRES_USER is required}"

echo "Waiting for PostgreSQL at ${POSTGRES_SEEDS}:${DB_PORT}..."
attempt=1
while ! nc -z -w 2 "${POSTGRES_SEEDS}" "${DB_PORT}"; do
  if [ "$attempt" -ge 30 ]; then
    echo "PostgreSQL did not become available" >&2
    exit 1
  fi
  attempt=$((attempt + 1))
  sleep 2
done

setup_database() {
  database="$1"
  schema="$2"
  echo "Setting up Temporal database: $database"
  temporal-sql-tool --plugin postgres12 --ep "${POSTGRES_SEEDS}" -u "${POSTGRES_USER}" -p "${DB_PORT}" --db "$database" create || true
  temporal-sql-tool --plugin postgres12 --ep "${POSTGRES_SEEDS}" -u "${POSTGRES_USER}" -p "${DB_PORT}" --db "$database" setup-schema -v 0.0
  temporal-sql-tool --plugin postgres12 --ep "${POSTGRES_SEEDS}" -u "${POSTGRES_USER}" -p "${DB_PORT}" --db "$database" update-schema -d "$schema"
}

setup_database "$DBNAME" /etc/temporal/schema/postgresql/v12/temporal/versioned
setup_database "$VISIBILITY_DBNAME" /etc/temporal/schema/postgresql/v12/visibility/versioned
echo "Temporal PostgreSQL schemas are ready."
