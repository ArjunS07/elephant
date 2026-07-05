# elephant

A podcast reverse proxy with transcription and hybrid semantic search.

## Architecture

- **Go server** (`cmd/elephant`) — proxies audio, serves feeds, exposes the search + transcript API.
- **Transcribe worker** (`worker/`) — drains the transcription queue, runs whisper, writes timestamped segments.
- **Embed worker** (`worker/`) — chunks segments, embeds them, writes searchable vectors.
- **Model box** (`modelbox/`) — stateless HTTP service holding the embedder + reranker.
- **Postgres** (pgvector) — source of truth and both search indexes.

Ingest chains automatically: streaming an episode past a threshold enqueues transcription → segments → embedding → chunks.

## Prerequisites

- Docker, Go, Python 3, ffmpeg
- Apple Silicon (the workers run whisper and embeddings on Metal/MPS)

## Setup

Create `.env` in the repo root:

```
DATABASE_URL=postgres://dev:dev@localhost:5432/elephant_dev
SECRET_KEY=<jwt signing key>
ENCRYPTION_KEY=<32-byte token encryption key>
```

## Run

```
./run-all.sh
```

Starts Postgres, the server (`:8080`), the model box (`:8081`), and both workers; Ctrl-C stops everything. Python venvs are built on first run.

Run pieces individually with `./run.sh` (server + DB), `./run-modelbox.sh`, `./run-worker.sh`, `./run-embedworker.sh`.

## API

- `POST /api/register`, `POST /api/login` — auth, returns a JWT.
- `POST /api/podcasts/register?feed_url=...` — subscribe to a feed (JWT).
- `GET /feeds/{slug}` — rewritten feed with proxied audio URLs.
- `POST /api/search` — hybrid search, episode-grouped + paginated (JWT).
- `GET /api/episodes/{id}/transcript` — full transcript (JWT).
