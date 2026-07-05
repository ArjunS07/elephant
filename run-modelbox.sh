#!/usr/bin/env bash
set -euo pipefail

# Runs the model box: the local embedder + reranker behind an HTTP API. Ensures
# the Python venv and deps exist, then execs uvicorn. Safe to run on its own or
# from run-all.sh.

cd "$(dirname "$0")/modelbox"

# 1. Create the venv and install deps on first run (no-op afterwards).
if [ ! -d .venv ]; then
  echo "creating modelbox venv..."
  python3 -m venv .venv
  ./.venv/bin/pip install --quiet --upgrade pip
  ./.venv/bin/pip install --quiet -r requirements.txt
fi

# 2. Load config (model names, host, port) from the repo-root .env.
set -a
source ../.env
set +a

# 3. Run the service (exec so Ctrl-C / signals reach it directly).
exec ./.venv/bin/uvicorn app:app \
  --host "${MODELBOX_HOST:-127.0.0.1}" --port "${MODELBOX_PORT:-8081}"
