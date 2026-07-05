"""Grouping transcript segments into chunks: the passages we embed and search.
No database or model access here, just the segment-to-chunk arithmetic."""

from config import CHUNK_MAX_CHARS, CHUNK_TARGET_SEC


def chunk_segments(segments):
    """Group adjacent segments into chunks, breaking only on segment boundaries
    (no overlap). A chunk closes once it holds ~CHUNK_TARGET_SEC of audio or
    CHUNK_MAX_CHARS of text, whichever comes first.

    segments: ordered (idx, start_ms, end_ms, text) from transcript_segments.
    Returns ordered (idx, start_ms, end_ms, text) chunks with a fresh 0-based idx.
    """
    chunks = []
    cur = []  # segments accumulated for the current chunk

    def flush():
        if not cur:
            return
        start_ms = cur[0][1]
        end_ms = cur[-1][2]
        text = " ".join(seg[3] for seg in cur)
        chunks.append((len(chunks), start_ms, end_ms, text))
        cur.clear()

    for seg in segments:
        cur.append(seg)
        duration_sec = (cur[-1][2] - cur[0][1]) / 1000
        chars = sum(len(s[3]) for s in cur)
        if duration_sec >= CHUNK_TARGET_SEC or chars >= CHUNK_MAX_CHARS:
            flush()

    flush()  # trailing segments that never crossed a threshold
    return chunks
