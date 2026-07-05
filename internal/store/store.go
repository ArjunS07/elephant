// Package store owns all database access. Handlers depend on *Store rather than
// touching the connection pool directly. Methods mirror the original dbOps
// functions verbatim (same SQL, same context.Background()); URL normalization
// and slug generation live in the feed package, so callers pass ready values.
package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) CreateUser(username, passwordHash string) (string, error) {
	var id string
	err := s.pool.QueryRow(context.Background(),
		"INSERT INTO users (username, password_hash) VALUES ($1, $2) RETURNING id",
		username, passwordHash).Scan(&id)
	return id, err
}

func (s *Store) UserByUsername(username string) (id, passwordHash string, err error) {
	err = s.pool.QueryRow(context.Background(),
		"SELECT id, password_hash FROM users WHERE username = $1",
		username).Scan(&id, &passwordHash)
	return id, passwordHash, err
}

// GetOrCreateShow stores the feed URL as given (used verbatim to fetch later);
// dedup is exact-match on it.
func (s *Store) GetOrCreateShow(feedURL string) (string, bool, error) {
	var id string
	var created bool
	err := s.pool.QueryRow(context.Background(),
		`INSERT INTO shows (feed_url) VALUES ($1)
		 ON CONFLICT (feed_url) DO UPDATE SET feed_url = EXCLUDED.feed_url
		 RETURNING id, (xmax = 0)`,
		feedURL).Scan(&id, &created)
	return id, created, err
}

// expects a pre-computed slug.
func (s *Store) GetOrCreateUserShow(userID, showID, slug string) (string, bool, error) {
	var friendlySlug string
	var created bool
	err := s.pool.QueryRow(context.Background(),
		`INSERT INTO user_shows (user_id, show_id, friendly_unique_slug)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (user_id, show_id) DO UPDATE SET friendly_unique_slug = user_shows.friendly_unique_slug
		 RETURNING friendly_unique_slug, (xmax = 0)`,
		userID, showID, slug).Scan(&friendlySlug, &created)
	return friendlySlug, created, err
}

// ShowIDsBySlug looks up the (userID, showID) behind a personal feed slug.
func (s *Store) ShowIDsBySlug(slug string) (userID, showID string, err error) {
	err = s.pool.QueryRow(context.Background(),
		"SELECT user_id, show_id FROM user_shows WHERE friendly_unique_slug = $1",
		slug).Scan(&userID, &showID)
	return userID, showID, err
}

func (s *Store) FeedURLByShowID(showID string) (string, error) {
	var feedURL string
	err := s.pool.QueryRow(context.Background(),
		"SELECT feed_url FROM shows WHERE id = $1", showID).Scan(&feedURL)
	return feedURL, err
}

// pubDate may be nil when the feed's date is absent or unparseable (stored NULL).
func (s *Store) GetOrCreateEpisode(showID, guid, guidSource, audioURL, title, description string, pubDate *time.Time) (string, error) {
	var id string
	err := s.pool.QueryRow(context.Background(),
		`INSERT INTO episodes (show_id, guid, guid_source, audio_url, title, description, pub_date)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 ON CONFLICT (show_id, guid) DO UPDATE SET
		     audio_url   = EXCLUDED.audio_url,
		     title       = EXCLUDED.title,
		     description = EXCLUDED.description,
		     pub_date    = EXCLUDED.pub_date
		 RETURNING id`,
		showID, guid, guidSource, audioURL, title, description, pubDate).Scan(&id)
	return id, err
}

func (s *Store) EpisodeAudioURL(episodeID string) (string, error) {
	var audioURL string
	err := s.pool.QueryRow(context.Background(),
		"SELECT audio_url FROM episodes WHERE id = $1", episodeID).Scan(&audioURL)
	return audioURL, err
}

func (s *Store) GetOrCreateUserEpisode(userID, episodeID string) (string, bool, error) {
	var id string
	var created bool
	err := s.pool.QueryRow(context.Background(),
		`INSERT INTO user_episodes (user_id, episode_id)
		 VALUES ($1, $2)
		 ON CONFLICT (user_id, episode_id) DO UPDATE SET last_played_at = NOW()
		 RETURNING id, (xmax = 0)`,
		userID, episodeID).Scan(&id, &created)
	return id, created, err
}

// AddPlaybackSeconds increments the user-episode's play time and returns the
// new cumulative total.
func (s *Store) AddPlaybackSeconds(userEpisodeID string, seconds int64) (int64, error) {
	var total int64
	err := s.pool.QueryRow(context.Background(),
		`UPDATE user_episodes
		    SET total_seconds_played = total_seconds_played + $1,
		        last_played_at = NOW()
		  WHERE id = $2
		  RETURNING total_seconds_played`,
		seconds, userEpisodeID).Scan(&total)
	return total, err
}

// EnqueueTranscription queues an episode for transcription. Idempotent: the
// queue is keyed by episode_id, so repeat calls are no-ops.
func (s *Store) EnqueueTranscription(episodeID string) error {
	_, err := s.pool.Exec(context.Background(),
		`INSERT INTO transcription_queue (episode_id) VALUES ($1)
		 ON CONFLICT (episode_id) DO NOTHING`,
		episodeID)
	return err
}
