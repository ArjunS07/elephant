"""The embed_queue lifecycle: claiming jobs, reading segments, saving chunks,
recording failures. Mirrors jobs.py, which does the same for transcription_queue."""

import logging

from config import LEASE_TIMEOUT_MIN, MAX_ATTEMPTS

log = logging.getLogger(__name__)


def claim_job(conn):
    """Take ownership of the next episode to embed, or return None if the queue is
    empty. Same claim as the transcribe worker: highest priority first, reclaiming
    'started' rows whose lease has expired, bumping attempts so a worker that dies
    before reporting failure still counts. Locks the row so multiple workers are
    safe."""
    return conn.execute(
        """
        UPDATE embed_queue
           SET status='started', locked_at=NOW(), started_at=NOW(), attempts=attempts+1
         WHERE episode_id = (
             SELECT episode_id FROM embed_queue
              WHERE status='pending'
                 OR (status='started' AND locked_at < NOW() - make_interval(mins => %s))
              ORDER BY priority DESC, created_at
              FOR UPDATE SKIP LOCKED
              LIMIT 1
         )
         RETURNING episode_id, attempts
        """,
        (LEASE_TIMEOUT_MIN,),
    ).fetchone()  # (episode_id, attempts) or None


def segments_for_episode(conn, episode_id):
    """The episode's transcript segments in order, the input to chunking."""
    return conn.execute(
        "SELECT idx, start_ms, end_ms, text FROM transcript_segments"
        " WHERE episode_id = %s ORDER BY idx",
        (episode_id,),
    ).fetchall()


def save_result(conn, episode_id, rows):
    """Persist an episode's chunks and clear the job, in one transaction. rows are
    (idx, start_ms, end_ms, text, embedding) with embedding a list of floats. The
    DELETE first clears any chunks from an earlier failed attempt."""
    with conn.transaction():
        conn.execute(
            "DELETE FROM transcript_chunks WHERE episode_id = %s", (episode_id,)
        )
        conn.cursor().executemany(
            "INSERT INTO transcript_chunks (episode_id, idx, start_ms, end_ms, text, embedding)"
            " VALUES (%s, %s, %s, %s, %s, %s)",
            [
                # halfvec accepts the pgvector text literal '[v1,v2,...]'.
                (episode_id, idx, start, end, text, "[" + ",".join(map(str, emb)) + "]")
                for (idx, start, end, text, emb) in rows
            ],
        )
        conn.execute("DELETE FROM embed_queue WHERE episode_id = %s", (episode_id,))


def fail_job(conn, episode_id, attempts, err):
    """Record a failed attempt. Below the retry limit the job goes back to
    'pending'; once exhausted it becomes 'failed' with the error. Only the queue
    row is touched -- chunk presence is what marks an episode embedded."""
    if attempts >= MAX_ATTEMPTS:
        log.error(
            "episode %s embedding failed permanently after %d attempts: %s",
            episode_id,
            attempts,
            err,
        )
        conn.execute(
            "UPDATE embed_queue SET status='failed', last_error=%s WHERE episode_id = %s",
            (err, episode_id),
        )
    else:
        log.warning(
            "episode %s embedding failed (attempt %d), will retry: %s",
            episode_id,
            attempts,
            err,
        )
        conn.execute(
            "UPDATE embed_queue SET status='pending', last_error=%s WHERE episode_id = %s",
            (err, episode_id),
        )
