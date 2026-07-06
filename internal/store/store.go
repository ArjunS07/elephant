// Package store owns all database access. Handlers depend on *Store rather than
// touching the connection pool directly. Methods mirror the original dbOps
// functions verbatim (same SQL, same context.Background()); URL normalization
// and slug generation live in the feed package, so callers pass ready values.
package store

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
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

// Chunk is one searchable passage: a run of transcript segments with its episode
// title and jump-to offsets.
type Chunk struct {
	EpisodeID string
	Idx       int
	StartMs   int
	EndMs     int
	Text      string
	Title     string
}

// Both searches restrict to chunks under shows the user is subscribed to, and
// return rows already in rank order (best first).

// SemanticSearch returns the nearest chunks to queryVec by cosine distance.
func (s *Store) SemanticSearch(userID string, queryVec []float32, limit int) ([]Chunk, error) {
	rows, err := s.pool.Query(context.Background(),
		`SELECT c.episode_id, c.idx, c.start_ms, c.end_ms, c.text, e.title
		   FROM transcript_chunks c
		   JOIN episodes e   ON e.id = c.episode_id
		   JOIN user_shows us ON us.show_id = e.show_id
		  WHERE us.user_id = $2
		  ORDER BY c.embedding <=> $1::halfvec
		  LIMIT $3`,
		vectorLiteral(queryVec), userID, limit)
	if err != nil {
		return nil, err
	}
	return scanChunks(rows)
}

// LexicalSearch returns chunks matching the query text, ranked by ts_rank. An
// empty result is normal (the query may have no lexical hits).
func (s *Store) LexicalSearch(userID, queryText string, limit int) ([]Chunk, error) {
	rows, err := s.pool.Query(context.Background(),
		`SELECT c.episode_id, c.idx, c.start_ms, c.end_ms, c.text, e.title
		   FROM transcript_chunks c
		   JOIN episodes e   ON e.id = c.episode_id
		   JOIN user_shows us ON us.show_id = e.show_id
		  WHERE us.user_id = $2
		    AND c.tsv @@ websearch_to_tsquery('english', $1)
		  ORDER BY ts_rank(c.tsv, websearch_to_tsquery('english', $1)) DESC
		  LIMIT $3`,
		queryText, userID, limit)
	if err != nil {
		return nil, err
	}
	return scanChunks(rows)
}

// EpisodeDurations returns each episode's full length in ms (its last chunk's
// end_ms), keyed by episode_id. The frontend draws the result bar against this.
func (s *Store) EpisodeDurations(episodeIDs []string) (map[string]int, error) {
	rows, err := s.pool.Query(context.Background(),
		`SELECT episode_id, MAX(end_ms) FROM transcript_chunks
		  WHERE episode_id = ANY($1) GROUP BY episode_id`,
		episodeIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var id string
		var dur int
		if err := rows.Scan(&id, &dur); err != nil {
			return nil, err
		}
		out[id] = dur
	}
	return out, rows.Err()
}

// Segment is one transcript line, the reading unit on the episode page.
type Segment struct {
	Idx     int
	StartMs int
	EndMs   int
	Text    string
}

// EpisodeMeta returns an episode's display fields, but only if the user is
// subscribed to its show. ok is false when the episode is unknown or unsubscribed
// (the caller turns that into a 404), matching how search is scoped.
func (s *Store) EpisodeMeta(userID, episodeID string) (title, description string, pubDate *time.Time, ok bool, err error) {
	err = s.pool.QueryRow(context.Background(),
		`SELECT COALESCE(e.title, ''), COALESCE(e.description, ''), e.pub_date
		   FROM episodes e
		   JOIN user_shows us ON us.show_id = e.show_id
		  WHERE e.id = $1 AND us.user_id = $2`,
		episodeID, userID).Scan(&title, &description, &pubDate)
	if err == pgx.ErrNoRows {
		return "", "", nil, false, nil
	}
	if err != nil {
		return "", "", nil, false, err
	}
	return title, description, pubDate, true, nil
}

// TranscriptSegments returns an episode's segments in order.
func (s *Store) TranscriptSegments(episodeID string) ([]Segment, error) {
	rows, err := s.pool.Query(context.Background(),
		"SELECT idx, start_ms, end_ms, text FROM transcript_segments WHERE episode_id = $1 ORDER BY idx",
		episodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Segment
	for rows.Next() {
		var seg Segment
		if err := rows.Scan(&seg.Idx, &seg.StartMs, &seg.EndMs, &seg.Text); err != nil {
			return nil, err
		}
		out = append(out, seg)
	}
	return out, rows.Err()
}

func scanChunks(rows pgx.Rows) ([]Chunk, error) {
	defer rows.Close()
	var out []Chunk
	for rows.Next() {
		var c Chunk
		if err := rows.Scan(&c.EpisodeID, &c.Idx, &c.StartMs, &c.EndMs, &c.Text, &c.Title); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// vectorLiteral formats a vector as pgvector's text form, '[v1,v2,...]', for
// binding into a halfvec column.
func vectorLiteral(v []float32) string {
	var b strings.Builder
	b.WriteByte('[')
	for i, x := range v {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(float64(x), 'f', -1, 32))
	}
	b.WriteByte(']')
	return b.String()
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

// UpdateShowMeta refreshes a show's display fields from a feed fetch. Empty
// values are ignored (NULLIF -> COALESCE) so a feed missing a field doesn't wipe
// what's already stored.
func (s *Store) UpdateShowMeta(showID, title, description, image string) error {
	_, err := s.pool.Exec(context.Background(),
		`UPDATE shows
		    SET title       = COALESCE(NULLIF($2, ''), title),
		        description = COALESCE(NULLIF($3, ''), description),
		        image_url   = COALESCE(NULLIF($4, ''), image_url),
		        last_fetched = NOW()
		  WHERE id = $1`,
		showID, title, description, image)
	return err
}

// UserShow is a show as it appears on a user's shows page. SourceFeedURL is the
// publisher's feed; the personal proxy URL is built from Slug by the handler.
type UserShow struct {
	ShowID        string
	Title         string
	Description   string
	ImageURL      string
	SourceFeedURL string
	Slug          string
	SubscribedAt  time.Time
}

const userShowCols = `s.id, COALESCE(s.title, ''), COALESCE(s.description, ''),
	COALESCE(s.image_url, ''), s.feed_url, us.friendly_unique_slug, us.subscribed_at`

func scanUserShow(row pgx.Row) (UserShow, error) {
	var us UserShow
	err := row.Scan(&us.ShowID, &us.Title, &us.Description, &us.ImageURL,
		&us.SourceFeedURL, &us.Slug, &us.SubscribedAt)
	return us, err
}

// ListUserShows returns the user's registered shows, newest subscription first.
func (s *Store) ListUserShows(userID string) ([]UserShow, error) {
	rows, err := s.pool.Query(context.Background(),
		`SELECT `+userShowCols+`
		   FROM user_shows us JOIN shows s ON s.id = us.show_id
		  WHERE us.user_id = $1
		  ORDER BY us.subscribed_at DESC`,
		userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []UserShow
	for rows.Next() {
		us, err := scanUserShow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, us)
	}
	return out, rows.Err()
}

// UserShowByID returns one of the user's shows. ok is false when the show is
// unknown or the user isn't subscribed (the caller turns that into a 404).
func (s *Store) UserShowByID(userID, showID string) (UserShow, bool, error) {
	us, err := scanUserShow(s.pool.QueryRow(context.Background(),
		`SELECT `+userShowCols+`
		   FROM user_shows us JOIN shows s ON s.id = us.show_id
		  WHERE us.user_id = $1 AND s.id = $2`,
		userID, showID))
	if err == pgx.ErrNoRows {
		return UserShow{}, false, nil
	}
	if err != nil {
		return UserShow{}, false, err
	}
	return us, true, nil
}

// UserEpisode is one episode in a user's listening history for a show.
type UserEpisode struct {
	EpisodeID          string
	Title              string
	PubDate            *time.Time
	TranscriptStatus   string
	FirstPlayedAt      *time.Time
	LastPlayedAt       *time.Time
	TotalSecondsPlayed int
}

// ShowEpisodes returns the episodes the user has played from a show, most
// recently played first.
func (s *Store) ShowEpisodes(userID, showID string) ([]UserEpisode, error) {
	rows, err := s.pool.Query(context.Background(),
		`SELECT e.id, COALESCE(e.title, ''), e.pub_date, e.transcript_status,
		        ue.first_played_at, ue.last_played_at, ue.total_seconds_played
		   FROM user_episodes ue JOIN episodes e ON e.id = ue.episode_id
		  WHERE ue.user_id = $1 AND e.show_id = $2
		  ORDER BY ue.last_played_at DESC`,
		userID, showID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []UserEpisode
	for rows.Next() {
		var e UserEpisode
		if err := rows.Scan(&e.EpisodeID, &e.Title, &e.PubDate, &e.TranscriptStatus,
			&e.FirstPlayedAt, &e.LastPlayedAt, &e.TotalSecondsPlayed); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
