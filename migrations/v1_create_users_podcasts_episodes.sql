
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    username VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE podcast_shows (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    feed_url TEXT UNIQUE NOT NULL,
    last_fetched TIMESTAMP,
    created_at TIMESTAMP DEFAULT NOW()
);
CREATE INDEX idx_podcasts_feed_url ON podcast_shows(feed_url);

CREATE TABLE user_podcast_shows(
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    friendly_unique_slug VARCHAR(128) UNIQUE NOT NULL,
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    podcast_show_id UUID REFERENCES podcast_shows(id) ON DELETE CASCADE,
    subscribed_at TIMESTAMP DEFAULT NOW(),
    UNIQUE(user_id, podcast_show_id)
);
CREATE INDEX idx_user_podcast_shows_user ON user_podcast_shows(user_id);
CREATE INDEX idx_user_podcast_shows_show ON user_podcast_shows(podcast_show_id);

-- Episodes (shared across all users)
CREATE TABLE episodes (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    podcast_id UUID REFERENCES podcast_shows(id) ON DELETE CASCADE,
    guid TEXT NOT NULL,
    title TEXT,
    description TEXT,
    pub_date TIMESTAMP,
    duration_seconds INT,
    audio_url TEXT NOT NULL,
    audio_size_bytes BIGINT,
    
    transcript_status VARCHAR(20) DEFAULT 'pending',
    -- pending, queued, processing, completed, failed
    transcript_full_text TEXT,
    transcribed_at TIMESTAMP,
    transcription_error TEXT,
    
    -- Storage
    audio_cached BOOLEAN DEFAULT false,
    audio_cache_path TEXT,
    cached_at TIMESTAMP,
    
    created_at TIMESTAMP DEFAULT NOW(),
    UNIQUE(podcast_id, guid)
);

CREATE INDEX idx_episodes_podcast ON episodes(podcast_id);
CREATE INDEX idx_episodes_status ON episodes(transcript_status);
CREATE INDEX idx_episodes_guid ON episodes(guid);

-- User listening history
CREATE TABLE user_episodes (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    episode_id UUID REFERENCES episodes(id) ON DELETE CASCADE,
    episode_guid TEXT NOT NULL,
    
    first_played_at TIMESTAMP DEFAULT NOW(),
    last_played_at TIMESTAMP DEFAULT NOW(),
    play_count INT DEFAULT 1,
    total_seconds_played INT DEFAULT 0,

    UNIQUE (user_id, episode_id)
);

CREATE INDEX idx_user_episodes_user ON user_episodes(user_id, last_played_at DESC);
CREATE INDEX idx_user_episodes_episode ON user_episodes(episode_id);
CREATE INDEX idx_user_episodes_guid ON user_episodes(user_id, episode_guid);