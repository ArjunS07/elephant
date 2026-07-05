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

    transcript_status    VARCHAR(20) NOT NULL DEFAULT 'pending',  -- pending|queued|processing|completed|failed
    transcript_full_text TEXT,
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

-- Transcription work queue (V3)
CREATE TABLE transcription_queue (
    episode_id         UUID PRIMARY KEY REFERENCES episodes(id) ON DELETE CASCADE,
    priority           INT NOT NULL DEFAULT 1,
    requested_by_count INT NOT NULL DEFAULT 1,
    status             VARCHAR(20) NOT NULL DEFAULT 'pending',
    started_at         TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_queue_priority ON transcription_queue(status, priority DESC, created_at);
