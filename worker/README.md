# Transcription worker

Standalone Python process that drains `transcription_queue`. Fetches the audio, runs whisper, and writes timestamped segments into `transcript_segments`.

## Prerequisites

- **ffmpeg** on PATH (used to transcode audio to 16 kHz mono WAV):
  ```
  brew install ffmpeg
  ```
- Apple Silicon (mlx-whisper runs on Metal).

## Setup

```
cd worker
python3 -m venv .venv
source .venv/bin/activate
pip install -r requirements.txt
```

## Run

The worker reads its config straight from the environment and does not parse the `.env` file itself. Export the same `DATABASE_URL` the Go server uses, e.g.:

```
export $(grep -v '^#' ../.env | xargs)
```

Optional overrides:

| Var                 | Default                                   |
|---------------------|-------------------------------------------|
| `DATABASE_URL`      | (required)                                |
| `WHISPER_MODEL`     | `mlx-community/whisper-large-v3-turbo`    |
| `POLL_INTERVAL_SEC` | `5`                                       |
| `MAX_ATTEMPTS`      | `3`                                       |
| `LEASE_TIMEOUT_MIN` | `30`                                      |

```
python transcribe.py
```

The `DATABASE_URL` in `.env` must be reachable from the host (the docker-compose Postgres maps to `localhost:5432`).

The worker loops forever until terminated.
