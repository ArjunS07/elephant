CREATE TABLE users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username      VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Shows (one row / feed)
CREATE TABLE shows (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    feed_url     TEXT UNIQUE NOT NULL,
    last_fetched TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- A user's subscription to a show (+ personal feed slug)
CREATE TABLE user_shows (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    friendly_unique_slug VARCHAR(128) UNIQUE NOT NULL,
    user_id              UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    show_id              UUID NOT NULL REFERENCES shows(id) ON DELETE CASCADE,
    subscribed_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, show_id)
);
CREATE INDEX idx_user_shows_user ON user_shows(user_id);
CREATE INDEX idx_user_shows_show ON user_shows(show_id);

-- Global episodes table, one row per (show, guid)
CREATE TABLE episodes (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    show_id     UUID NOT NULL REFERENCES shows(id) ON DELETE CASCADE,
    guid        TEXT NOT NULL,                      -- 'synthetic:<hash>' when publisher omits one
    guid_source TEXT NOT NULL DEFAULT 'feed',       -- 'feed' | 'synthetic'

    title            TEXT,
    description      TEXT,
    pub_date         TIMESTAMPTZ,
    duration_seconds INT,
    audio_url        TEXT NOT NULL,                 -- mutable, refreshed on each fetch
    audio_size_bytes BIGINT,

    transcript_status    VARCHAR(20) NOT NULL DEFAULT 'pending',  -- pending|processing|completed|failed
    transcript_full_text TEXT,                                     -- derived copy; transcript_segments is authoritative
    transcribed_at       TIMESTAMPTZ,
    transcription_error  TEXT,

    audio_cached     BOOLEAN NOT NULL DEFAULT false,
    audio_cache_path TEXT,
    cached_at        TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (show_id, guid)
);
CREATE INDEX idx_episodes_status ON episodes(transcript_status);

-- Per-user listening history for episodes
CREATE TABLE user_episodes (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id              UUID NOT NULL REFERENCES users(id)    ON DELETE CASCADE,
    episode_id           UUID NOT NULL REFERENCES episodes(id) ON DELETE CASCADE,
    first_played_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_played_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    total_seconds_played INT NOT NULL DEFAULT 0,
    UNIQUE (user_id, episode_id)
);
CREATE INDEX idx_user_episodes_user    ON user_episodes(user_id, last_played_at DESC);
CREATE INDEX idx_user_episodes_episode ON user_episodes(episode_id);

-- ============================================================================
-- V3 TRANSCRIPTION
--
-- The Go proxy enqueues an episode here once ~60s of playback streams through
-- it. A separate Python worker (worker/transcribe.py) polls this queue, fetches
-- the audio, runs whisper, and writes the timestamped result.
--
-- Source of truth for the transcript is transcript_segments (below); the
-- episodes.transcript_full_text column is only a derived convenience copy.
-- The queue holds work-in-flight only: on success its row is deleted and the
-- durable state lives on episodes.transcript_status.
-- ============================================================================

-- Transcription work queue. status: 'pending' -> 'started' -> ('failed' after
-- max attempts). Deleted on success.
CREATE TABLE transcription_queue (
    episode_id         UUID PRIMARY KEY REFERENCES episodes(id) ON DELETE CASCADE,
    priority           INT NOT NULL DEFAULT 1,
    requested_by_count INT NOT NULL DEFAULT 1,
    status             VARCHAR(20) NOT NULL DEFAULT 'pending',
    attempts           INT NOT NULL DEFAULT 0,       -- bumped on each claim
    last_error         TEXT,                          -- why the last attempt failed
    locked_at          TIMESTAMPTZ,                   -- lease; stale 'started' rows get reclaimed
    started_at         TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_queue_priority ON transcription_queue(status, priority DESC, created_at);

-- Timestamped transcript segments: the authoritative transcript artifact.
-- One row per whisper segment, in order, with start/end offsets in integer ms.
-- Downstream chunking snaps to these boundaries; the timestamps are the
-- jump-to-moment payload.
CREATE TABLE transcript_segments (
    episode_id UUID NOT NULL REFERENCES episodes(id) ON DELETE CASCADE,
    idx        INT  NOT NULL,          -- 0-based order within the episode
    start_ms   INT  NOT NULL,
    end_ms     INT  NOT NULL,
    text       TEXT NOT NULL,
    PRIMARY KEY (episode_id, idx)
);
