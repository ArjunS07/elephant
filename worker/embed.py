"""Embed worker entrypoint.

Polls embed_queue and, for each claimed episode, turns its transcript segments
into embedded chunks in transcript_chunks. Talks to Postgres and the model box;
independent of the Go server. Run: python embed.py   (see README.md for env vars)
"""

import sys
import time
import logging

import psycopg

from config import DATABASE_URL, MODELBOX_URL, POLL_INTERVAL_SEC
from embed_jobs import claim_job, segments_for_episode, save_result, fail_job
from chunking import chunk_segments
from embedding import embed_chunks

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s %(levelname)s %(message)s",
)
log = logging.getLogger("embed")


def process_job(conn, episode_id):
    segments = segments_for_episode(conn, episode_id)
    chunks = chunk_segments(segments)
    if not chunks:
        log.warning("episode %s has no segments to embed", episode_id)
        save_result(conn, episode_id, [])
        return

    vectors = embed_chunks([text for _, _, _, text in chunks])
    rows = [(idx, start, end, text, vec) for (idx, start, end, text), vec in zip(chunks, vectors)]
    save_result(conn, episode_id, rows)
    log.info("episode %s embedded: %d segments -> %d chunks", episode_id, len(segments), len(chunks))


def main():
    if not DATABASE_URL:
        log.error("DATABASE_URL is required")
        sys.exit(1)

    log.info("embed worker starting (modelbox=%s, poll=%ds)", MODELBOX_URL, POLL_INTERVAL_SEC)
    with psycopg.connect(DATABASE_URL, autocommit=True) as conn:
        while True:
            job = claim_job(conn)
            if job is None:
                time.sleep(POLL_INTERVAL_SEC)
                continue
            episode_id, attempts = job
            try:
                process_job(conn, episode_id)
            except Exception as e:
                fail_job(conn, episode_id, attempts, str(e))


if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        log.info("shutting down")
