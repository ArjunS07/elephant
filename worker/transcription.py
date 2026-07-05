"""Turning one episode's audio into timestamped segments: fetch the audio,
transcode it for whisper, and run the model."""

import os
import logging
import subprocess
import tempfile
import urllib.request

import mlx_whisper

from config import WHISPER_MODEL

log = logging.getLogger(__name__)


def episode_audio_url(conn, episode_id):
    row = conn.execute(
        "SELECT audio_url FROM episodes WHERE id = %s", (episode_id,)
    ).fetchone()
    if row is None:
        raise RuntimeError(f"episode {episode_id} not found")
    return row[0]


def download(url, dest_path):
    # Some CDNs reject clients without a User-Agent.
    req = urllib.request.Request(url, headers={"User-Agent": "elephant-transcriber/1.0"})
    with urllib.request.urlopen(req, timeout=60) as resp, open(dest_path, "wb") as f:
        while chunk := resp.read(1 << 16):
            f.write(chunk)


def to_wav(src_path, wav_path):
    # Mono, 16 kHz: whisper's native input format.
    subprocess.run(
        ["ffmpeg", "-y", "-i", src_path, "-ac", "1", "-ar", "16000", wav_path],
        check=True,
        stdout=subprocess.DEVNULL,
    )


def transcribe_episode(conn, episode_id):
    """Download the episode audio and return an ordered list of
    (idx, start_ms, end_ms, text) segments. The audio lives in a temp dir that
    is deleted on return."""
    audio_url = episode_audio_url(conn, episode_id)

    with tempfile.TemporaryDirectory(prefix="elephant-") as tmp:
        audio_path = os.path.join(tmp, "audio")
        wav_path = os.path.join(tmp, "audio.wav")

        log.info("downloading %s", audio_url)
        download(audio_url, audio_path)
        to_wav(audio_path, wav_path)

        log.info("transcribing episode %s", episode_id)
        result = mlx_whisper.transcribe(wav_path, path_or_hf_repo=WHISPER_MODEL)

    # whisper reports seconds as floats; we store integer milliseconds. Skip
    # empty segments, which whisper emits when it hallucinates on trailing
    # silence, and reindex so idx stays contiguous.
    segments = []
    for seg in result["segments"]:
        text = seg["text"].strip()
        if not text:
            continue
        segments.append(
            (len(segments), round(seg["start"] * 1000), round(seg["end"] * 1000), text)
        )
    return segments
