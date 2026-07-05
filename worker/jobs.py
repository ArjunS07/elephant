"""The transcription_queue lifecycle: claiming jobs, saving results, recording
failures. Everything that touches the queue table lives here."""

import logging

from config import LEASE_TIMEOUT_MIN, MAX_ATTEMPTS

log = logging.getLogger(__name__)


def claim_job(conn):
    """Take ownership of the next episode to transcribe, or return None if the
    queue is empty.

    Looks for the highest-priority job that is still 'pending', or that a
    previous worker marked 'started' but never finished. If if locked_at lease
    is older than LEASE_TIMEOUT_MIN we assume that worker crashed, so
    it is safe to pick the job back up.

    Can run more than one worker at a time since each worker locks the row it selects.

    Bump attempts on claim so a worker which dies before reporting failure still counts,
    and a permanently broken episode eventually gives up.
    """
    return conn.execute(
        """
        UPDATE transcription_queue
           SET status='started', locked_at=NOW(), started_at=NOW(), attempts=attempts+1
         WHERE episode_id = (
             SELECT episode_id FROM transcription_queue
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


def save_result(conn, episode_id, segments):
    """Persist a finished transcript and clear the job, in one transaction so a
    reader never sees a half-written result. The DELETE first clears any
    segments left by an earlier failed attempt."""
    full_text = " ".join(text for _, _, _, text in segments)
    with conn.transaction():
        conn.execute(
            "DELETE FROM transcript_segments WHERE episode_id = %s", (episode_id,)
        )
        conn.cursor().executemany(
            "INSERT INTO transcript_segments (episode_id, idx, start_ms, end_ms, text)"
            " VALUES (%s, %s, %s, %s, %s)",
            [(episode_id, i, start, end, text) for (i, start, end, text) in segments],
        )
        conn.execute(
            "UPDATE episodes"
            " SET transcript_status='completed', transcribed_at=NOW(),"
            "     transcript_full_text=%s, transcription_error=NULL"
            " WHERE id = %s",
            (full_text, episode_id),
        )
        conn.execute(
            "DELETE FROM transcription_queue WHERE episode_id = %s", (episode_id,)
        )


def fail_job(conn, episode_id, attempts, err):
    """Record a failed attempt. Below the retry limit the job goes back to
    'pending' to be picked up again; once it is exhausted the failure becomes
    permanent on both the queue and the episode."""
    if attempts >= MAX_ATTEMPTS:
        log.error(
            "episode %s failed permanently after %d attempts: %s",
            episode_id,
            attempts,
            err,
        )
        with conn.transaction():
            conn.execute(
                "UPDATE transcription_queue SET status='failed', last_error=%s WHERE episode_id = %s",
                (err, episode_id),
            )
            conn.execute(
                "UPDATE episodes SET transcript_status='failed', transcription_error=%s WHERE id = %s",
                (err, episode_id),
            )
    else:
        log.warning(
            "episode %s failed (attempt %d), will retry: %s", episode_id, attempts, err
        )
        conn.execute(
            "UPDATE transcription_queue SET status='pending', last_error=%s WHERE episode_id = %s",
            (err, episode_id),
        )
