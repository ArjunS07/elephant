#!/usr/bin/env bash
set -euo pipefail

# Run from the repo root regardless of where this is invoked from, so the
# relative paths below (docker-compose.yml, migrations/, cmd/elephant, .env) resolve.
cd "$(dirname "$0")"

# 1. Ensure the Docker daemon is running
if ! docker info >/dev/null 2>&1; then
  echo "Docker daemon not running, starting..."
  open -a Docker
  printf "waiting for Docker"
  until docker info >/dev/null 2>&1; do printf "."; sleep 2; done
  echo " ready."
fi

# 2. Start Postgres (defined in docker-compose.yml).
docker compose up -d

# 3. Wait for Postgres to accept connections.
printf "waiting for Postgres"
until docker compose exec -T db pg_isready -U dev -d elephant_dev >/dev/null 2>&1; do
  printf "."; sleep 1
done
echo " ready."

# 4. Apply the schema on first run (no-op if the tables already exist).
if ! docker compose exec -T db psql -U dev -d elephant_dev -tAc "SELECT to_regclass('public.users')" 2>/dev/null | grep -q users; then
  echo "applying schema..."
  docker compose exec -T db psql -U dev -d elephant_dev -v ON_ERROR_STOP=1 < migrations/schema.sql
fi

# 5. Run the server (exec so Ctrl-C / signals reach it directly).
exec go run ./cmd/elephant
