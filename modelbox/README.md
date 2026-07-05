# Model box

Stateless HTTP service holding the two search models: the `bge-base` embedder and
the `bge-reranker` cross-encoder. Go (query time) and the embed worker (ingest)
both call it, so the models are resident once. It never touches the database.

## Prerequisites

- Apple Silicon for MPS (falls back to CPU via `DEVICE=cpu`).
- Models download from Hugging Face on first use (cached in `~/.cache`).

## Setup

```
cd modelbox
python3 -m venv .venv
source .venv/bin/activate
pip install -r requirements.txt
```

## Run

```
uvicorn app:app --host 127.0.0.1 --port 8081
```

Or use `../run-modelbox.sh`, which builds the venv and reads config from `../.env`.

## Config

| Var             | Default                   |
|-----------------|---------------------------|
| `EMBED_MODEL`   | `BAAI/bge-base-en-v1.5`   |
| `RERANK_MODEL`  | `BAAI/bge-reranker-base`  |
| `MODELBOX_HOST` | `127.0.0.1`               |
| `MODELBOX_PORT` | `8081`                    |
| `DEVICE`        | `mps`                     |

## API

- `POST /embed` — `{"texts": [...], "is_query": false}` → `{"embeddings": [[...]]}`
- `POST /rerank` — `{"query": "...", "passages": [...]}` → `{"scores": [...]}`
- `GET /health` — warms both models, returns their names.
