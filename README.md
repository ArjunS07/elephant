# elephant

## Development instructions
### Prerequisites

- Docker, Go, Python 3, ffmpeg
- Apple Silicon server (the workers run whisper and embeddings on Metal/MPS)

### Setup

Create `.env` in the repo root:

```
DATABASE_URL=postgres://dev:dev@localhost:5432/elephant_dev
SECRET_KEY=<jwt signing key>
ENCRYPTION_KEY=<32-byte token encryption key>
```

### Run

```
./run-all.sh
```

This:
- Starts Postgres
- The server (`:8080`)
- The model box (`:8081`)
- Both workers

`venv`s are built on the first run.

Components can be run individually as:
- Server and DB: `./run.sh`
- Model box: `./run-modelbox.sh`
- Workers: `./run-worker.sh`, `./run-embedworker.sh`.

### Frontend

Vue 3 + Vite, in `frontend/`.

```
cd frontend
npm install
npm run dev     # dev server on :5173, proxies /api to the Go server on :8080
npm run build   # emits dist/, which the Go server serves at :8080
```

In dev, use `:5173` (hot reload, no CORS via the proxy). In prod, run
`npm run build` and hit the Go server directly.

### API
- `POST /api/register`, `POST /api/login`
- `POST /api/podcasts/register?feed_url=...`
- `GET /feeds/{slug}`
- `POST /api/search`
- `GET /api/episodes/{id}/transcript`
