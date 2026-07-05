"""Worker configuration, read from the environment. See README.md."""

import os

DATABASE_URL = os.environ.get("DATABASE_URL")
WHISPER_MODEL = os.environ.get("WHISPER_MODEL", "mlx-community/whisper-large-v3-turbo")
POLL_INTERVAL_SEC = int(os.environ.get("POLL_INTERVAL_SEC", "5"))
MAX_ATTEMPTS = int(os.environ.get("MAX_ATTEMPTS", "3"))
LEASE_TIMEOUT_MIN = int(os.environ.get("LEASE_TIMEOUT_MIN", "30"))
