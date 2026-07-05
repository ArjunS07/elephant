#!/usr/bin/env bash
set -euo pipefail

# Starts the whole local stack: the Go server and the transcription worker, side
# by side. Ctrl-C stops both. Each sibling script owns its own setup (run.sh
# brings up Postgres and applies the schema; run-worker.sh builds the venv and
# waits for the DB), so this just launches and supervises them.

cd "$(dirname "$0")"

# Kill the whole process group on exit so both children -- and their
# descendants, like `go run`'s compiled binary -- go down together.
trap 'kill 0' SIGINT SIGTERM EXIT

./run.sh &
./run-modelbox.sh &
./run-worker.sh &
./run-embedworker.sh &

wait
