import os

EMBED_MODEL = os.environ.get("EMBED_MODEL", "BAAI/bge-base-en-v1.5")
RERANK_MODEL = os.environ.get("RERANK_MODEL", "BAAI/bge-reranker-base")

HOST = os.environ.get("MODELBOX_HOST", "127.0.0.1")
PORT = int(os.environ.get("MODELBOX_PORT", "8081"))

# "mps" on Apple Silicon, "cpu" elsewhere.
DEVICE = os.environ.get("DEVICE", "mps")
