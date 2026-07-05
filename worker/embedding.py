"""Client for the model box's /embed endpoint. The worker owns chunking and the
database; the actual embedding model lives in the separate model box process."""

import json
import urllib.request

from config import MODELBOX_URL


def embed_chunks(texts):
    """Embed chunk passages (not queries) and return one vector per text."""
    body = json.dumps({"texts": texts, "is_query": False}).encode()
    req = urllib.request.Request(
        f"{MODELBOX_URL}/embed",
        data=body,
        headers={"Content-Type": "application/json"},
    )
    with urllib.request.urlopen(req, timeout=120) as resp:
        return json.load(resp)["embeddings"]
