"""Transcription worker entrypoint.

Polls transcription_queue and, for each claimed episode, produces a timestamped
transcript in transcript_segments. Independent of the Go server; they share only
Postgres. Run: python transcribe.py   (see README.md for env vars)
"""

import sys
import time
import logging

import psycopg

from config import DATABASE_URL, WHISPER_MODEL, POLL_INTERVAL_SEC
from jobs import claim_job, save_result, fail_job
from transcription import transcribe_episode

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s %(levelname)s %(message)s",
)
log = logging.getLogger("transcribe")


def process_job(conn, episode_id, attempts):
    conn.execute("UPDATE episodes SET transcript_status='processing' WHERE id = %s", (episode_id,))
    segments = transcribe_episode(conn, episode_id)
    log.info("episode %s produced %d segments", episode_id, len(segments))
    save_result(conn, episode_id, segments)
    log.info("episode %s completed", episode_id)


def main():
    if not DATABASE_URL:
        log.error("DATABASE_URL is required")
        sys.exit(1)

    log.info("worker starting (model=%s, poll=%ds)", WHISPER_MODEL, POLL_INTERVAL_SEC)
    with psycopg.connect(DATABASE_URL, autocommit=True) as conn:
        while True:
            job = claim_job(conn)
            if job is None:
                time.sleep(POLL_INTERVAL_SEC)
                continue
            episode_id, attempts = job
            try:
                process_job(conn, episode_id, attempts)
            except Exception as e:
                fail_job(conn, episode_id, attempts, str(e))


if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        log.info("shutting down")
