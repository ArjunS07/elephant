#!/usr/bin/env bash
set -euo pipefail

# Runs the embed worker: drains embed_queue into transcript_chunks, calling the
# model box for vectors. Shares the transcribe worker's venv (worker/). Safe to
# run on its own or from run-all.sh.

cd "$(dirname "$0")/worker"

# 1. Create the venv and install deps on first run (no-op afterwards).
if [ ! -d .venv ]; then
  echo "creating worker venv..."
  python3 -m venv .venv
  ./.venv/bin/pip install --quiet --upgrade pip
  ./.venv/bin/pip install --quiet -r requirements.txt
fi

# 2. Load config (DATABASE_URL, MODELBOX_URL etc.) from the repo-root .env.
set -a
source ../.env
set +a

# 3. Wait for Postgres to accept connections before starting.
printf "embed worker waiting for Postgres"
until docker compose -f ../docker-compose.yml exec -T db pg_isready -U dev -d elephant_dev >/dev/null 2>&1; do
  printf "."; sleep 1
done
echo " ready."

# 4. Run the worker (exec so Ctrl-C / signals reach it directly).
exec ./.venv/bin/python embed.py
