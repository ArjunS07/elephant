"""Worker configuration, read from the environment. See README.md."""

import os

DATABASE_URL = os.environ.get("DATABASE_URL")
WHISPER_MODEL = os.environ.get("WHISPER_MODEL", "mlx-community/whisper-large-v3-turbo")
POLL_INTERVAL_SEC = int(os.environ.get("POLL_INTERVAL_SEC", "5"))
MAX_ATTEMPTS = int(os.environ.get("MAX_ATTEMPTS", "3"))
LEASE_TIMEOUT_MIN = int(os.environ.get("LEASE_TIMEOUT_MIN", "30"))

# Embed worker.
MODELBOX_URL = os.environ.get("MODELBOX_URL", "http://127.0.0.1:8081")
CHUNK_TARGET_SEC = int(os.environ.get("CHUNK_TARGET_SEC", "40"))
CHUNK_MAX_CHARS = int(os.environ.get("CHUNK_MAX_CHARS", "1500"))
