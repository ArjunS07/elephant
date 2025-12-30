package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/jwtauth/v5"
	"github.com/gosimple/slug"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/patrickmn/go-cache"
)

var dbpool *pgxpool.Pool
var feedCache *cache.Cache
var tokenAuth *jwtauth.JWTAuth

func main() {
	// setup
	var err error

	err = godotenv.Load()
	if err != nil {
		log.Println("No .env file found or error loading .env file")
	}

	dbpool, err = pgxpool.New(context.Background(), os.Getenv(("DATABASE_URL")))
	if err != nil {
		log.Fatalf("Unable to create pool: %v\n", err)
	}
	defer dbpool.Close()

	feedCache = cache.New(30*time.Minute, 10*time.Minute)
	SECRET_KEY := os.Getenv("SECRET_KEY")
	tokenAuth = jwtauth.New("HS256", []byte(SECRET_KEY), nil)

	// routes
	r := chi.NewRouter()
	r.Post("/api/register", registerHandler)
	r.Post("/api/login", loginHandler)

	r.Group(func(r chi.Router) {
		r.Use(jwtauth.Verifier(tokenAuth))
		r.Use(jwtauth.Authenticator(tokenAuth))
		r.Post("/api/podcasts/register", registerPodcastHandler)
	})

	r.Get("/feeds/{userPodcastUniqueSlug}", podcastFeedHandler)
	r.Get("/media/{token}/stream.mp3", mediaStreamingProxyHandler)

	log.Println("Server starting on :8080")
	log.Fatal(http.ListenAndServe(":8080", r))
}

func getUserId(r *http.Request) string {
	_, claims, _ := jwtauth.FromContext(r.Context())
	userID := claims["user_id"]
	if userID == nil {
		return ""
	}
	return userID.(string)
}

func registerPodcastHandler(w http.ResponseWriter, r *http.Request) {
	userId := getUserId(r)
	if userId == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	feedURL := r.URL.Query().Get("feed_url")
	if feedURL == "" {
		http.Error(w, "feed_url parameter required", http.StatusBadRequest)
		return
	}

	// assert that we can fetch from the URL
	resp, err := http.Get(feedURL)
	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			fmt.Printf("StatusCode: %d\n", resp.StatusCode)
		}
		fmt.Printf("Error fetching feed: %v\n", err)
		http.Error(w, "failed to fetch podcast feed from provided URL", http.StatusBadRequest)
		return
	}

	showId, created, err := getOrCreateShow(dbpool, feedURL)
	if err != nil {
		log.Printf("Error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	slug, created, err := getOrCreateUserPodcast(dbpool, userId, showId, feedURL)
	url := getUserShowFeedUrl(slug)
	if created {
		fmt.Fprintf(w, `{"message": "Created podcast", "url": "%s"}`, url)
	} else {
		fmt.Fprintf(w, `{"message": "Podcast already registered", "url": "%s"}`, url)
	}
}

func getOrCreateShow(dbpool *pgxpool.Pool, feedUrl string) (string, bool, error) {
	cleanedUrl := cleanUpUrl(feedUrl)

	var showId string
	err := dbpool.QueryRow(context.Background(), "SELECT id FROM podcast_shows WHERE feed_url = $1", cleanedUrl).Scan(&showId)
	if err == nil {
		// found the entry
		return showId, false, nil
	}

	if err.Error() != "no rows in result set" {
		// some other real error
		return "", false, err
	}

	// no existing entry, create one
	err = dbpool.QueryRow(context.Background(), "INSERT INTO podcast_shows (feed_url) VALUES ($1) RETURNING id", feedUrl).Scan(&showId)
	if err != nil {
		return "", false, err
	}
	return showId, true, nil
}

func getOrCreateUserPodcast(dbpool *pgxpool.Pool, userId string, showId string, showFeedUrl string) (string, bool, error) {
	var friendlyUniqueSlug string
	err := dbpool.QueryRow(context.Background(), "SELECT friendly_unique_slug FROM user_podcast_shows WHERE user_id = $1 AND podcast_show_id = $2", userId, showId).Scan(&friendlyUniqueSlug)
	if err == nil {
		// found the entry
		return friendlyUniqueSlug, false, nil
	}

	if err.Error() != "no rows in result set" {
		// some other real error
		return "", false, err
	}

	// no existing entry, create one
	friendlyUniqueSlug = genFriendlyUniqueSlug(userId, showId, showFeedUrl)
	err = dbpool.QueryRow(context.Background(), "INSERT INTO user_podcast_shows (user_id, podcast_show_id, friendly_unique_slug) VALUES ($1, $2, $3) RETURNING friendly_unique_slug", userId, showId, friendlyUniqueSlug).Scan(&friendlyUniqueSlug)
	if err != nil {
		return "", false, err
	}
	return friendlyUniqueSlug, true, nil
}

func genFriendlyUniqueSlug(userId string, showId string, showUrl string) string {
	// cap to 32 characters
	feedUrlSlug := slug.Make(cleanUpUrl(showUrl))
	feedUrlSlug = feedUrlSlug[:min(32, len(feedUrlSlug))]

	// take 32 characters of hash
	hashBytes := sha256.Sum256([]byte(userId + showId))
	hashSuffix := fmt.Sprintf("%x", hashBytes)[:8]

	return fmt.Sprintf("%s-%s", feedUrlSlug, hashSuffix)
}

func getUserShowFeedUrl(slug string) string {
	return fmt.Sprintf("%s/feeds/%s", getPublicHost(), slug)
}

func podcastFeedHandler(w http.ResponseWriter, r *http.Request) {
	userPodcastUniqueSlug := chi.URLParam(r, "userPodcastUniqueSlug")

	if cached, found := feedCache.Get(userPodcastUniqueSlug); found {
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=1800")
		w.Write(cached.([]byte))
		return
	}

	var showId string
	var userId string
	err := dbpool.QueryRow(context.Background(), "SELECT user_id, podcast_show_id from user_podcast_shows WHERE friendly_unique_slug = $1", userPodcastUniqueSlug).Scan(&userId, &showId)
	if err != nil {
		http.Error(w, "podcast not found for this user", http.StatusNotFound)
		return
	}

	var feedUrl string
	err = dbpool.QueryRow(context.Background(), "SELECT feed_url from podcast_shows WHERE id = $1", showId).Scan(&feedUrl)
	if err != nil {
		http.Error(w, "podcast feed not found", http.StatusNotFound)
		return
	}

	// Fetch original feed
	resp, err := http.Get(feedUrl)
	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			fmt.Printf("StatusCode: %d\n", resp.StatusCode)
		}
		fmt.Printf("Error fetching feed: %v\n", err)
		http.Error(w, "failed to fetch podcast feed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	feedContent, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "failed to read feed", http.StatusInternalServerError)
		return
	}

	modifiedFeed := rewriteEnclosureURLs(string(feedContent), userId, showId)

	feedCache.Set(userPodcastUniqueSlug, []byte(modifiedFeed), cache.DefaultExpiration)
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=1800")
	w.Write([]byte(modifiedFeed))
}

func rewriteEnclosureURLs(feedXML string, userId string, podcastId string) string {
	re := regexp.MustCompile(`(?s)<guid[^>]*>\s*(?:<!\[CDATA\[\s*([^\]]+)\s*\]\]>|([^<]+))\s*</guid>.*?(<enclosure[^>]+url=["']([^"']+)["'][^>]*>)`)

	return re.ReplaceAllStringFunc(feedXML, func(match string) string {
		submatch := re.FindStringSubmatch(match)
		if len(submatch) < 5 {
			return match
		}

		guid := strings.TrimSpace(submatch[1])
		if guid == "" {
			guid = strings.TrimSpace(submatch[2])
		}
		enclosureTag := submatch[3]
		originalURL := submatch[4]

		token, err := generateMediaToken(userId, guid, podcastId)
		if err != nil {
			log.Printf("Failed to generate token: %v", err)
			return match
		}

		encodedURL := url.QueryEscape(originalURL)

		proxyURL := fmt.Sprintf("%s/media/%s/stream.mp3?url=%s",
			getPublicHost(),
			token,
			encodedURL)

		newEnclosureTag := strings.Replace(enclosureTag, originalURL, proxyURL, 1)
		return strings.Replace(match, enclosureTag, newEnclosureTag, 1)
	})
}

func mediaStreamingProxyHandler(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")

	targetURLStr := r.URL.Query().Get("url")
	if targetURLStr == "" {
		http.Error(w, "Missing media URL", http.StatusBadRequest)
		return
	}
	targetURL, err := url.Parse(targetURLStr)
	if err != nil {
		http.Error(w, "Invalid target URL", http.StatusBadRequest)
		return
	}

	userId, episodeGuid, podcastId, err := decryptMediaToken(token)
	if err != nil {
		http.Error(w, "Invalid token", 400)
		return
	}

	// get or create user episode activity in DB with these Ids
	var userEpisodeId string
	userEpisodeId, _, err = getOrCreateUserEpisode(dbpool, userId, episodeGuid, podcastId, targetURL.String())
	if err != nil {
		log.Printf("Error getting/creating user episode: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	log.Printf("User %s accessing episode %s (user_episode_id: %s)", userId, episodeGuid, userEpisodeId)

	// proxy stream
	// 1. Handle HEAD requests explicitly
	// Podcast apps use this to check file size without downloading
	if r.Method == "HEAD" {
		handleHeadRequest(w, r, targetURL.String())
		return
	}

	client := &http.Client{
		// 0 means no timeout, which is dangerous for production.
		// Better to use a long timeout (e.g., 2 hours) or rely on req context.
		Timeout: 0,
		// We do NOT want to automatically decompress if we are just proxying bytes.
		// However, Go's transport does this automatically.
		// For audio files, this is rarely an issue as they aren't gzipped.
	}

	req, err := http.NewRequestWithContext(r.Context(), "GET", targetURL.String(), nil)
	if err != nil {
		http.Error(w, "Failed to create request", http.StatusInternalServerError)
		return
	}

	// 2. Forward Essential Headers
	copyHeader(r.Header, req.Header, "Range")
	copyHeader(r.Header, req.Header, "User-Agent")

	// 3. Analytics: Forward the real user IP
	// Podcasters rely on this for IAB v2 certification.
	if clientIP := r.Header.Get("X-Forwarded-For"); clientIP != "" {
		req.Header.Set("X-Forwarded-For", clientIP+", "+r.RemoteAddr)
	} else {
		req.Header.Set("X-Forwarded-For", r.RemoteAddr)
	}

	resp, err := client.Do(req)
	if err != nil {
		// Suppress logs for client disconnects
		if r.Context().Err() != nil {
			return
		}
		log.Printf("Proxy error: %v", err)
		http.Error(w, "Upstream error", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// 4. Careful Header Copying
	// We strictly copy Content-Type and Content-Length to ensure
	// the player knows the file size/duration.
	headersToForward := []string{
		"Content-Type",
		"Content-Length",
		"Content-Range",
		"Accept-Ranges",
		"Last-Modified",
		"ETag",
		"Cache-Control",
	}

	for _, h := range headersToForward {
		if v := resp.Header.Get(h); v != "" {
			w.Header().Set(h, v)
		}
	}

	w.WriteHeader(resp.StatusCode)

	// 5. Stream the data
	counter := &byteCounter{ResponseWriter: w}

	// 5. Stream the data
	_, err = io.Copy(counter, resp.Body)

	// Even if there's an error (like a disconnect), we record what was sent
	if counter.count > 0 {
		bitrate := ASSUMED_BITRATE_KBPS * 1000 // in bits per second
		secondsPlayed := (counter.count * 8) / int64(bitrate)

		_, dbErr := dbpool.Exec(context.Background(),
			`UPDATE user_episodes 
     SET total_seconds_played = total_seconds_played + $1,
         last_played_at = NOW()
     WHERE id = $2`,
			secondsPlayed, userEpisodeId)
		if dbErr != nil {
			log.Printf("Failed to update progress: %v", dbErr)
		}
	}
}

func getOrCreateEpisode(dbpool *pgxpool.Pool, episodeGuid string, podcastId string, mediaUrl string) (string, bool, error) {
	var episodeId string
	err := dbpool.QueryRow(context.Background(), "SELECT id from episodes WHERE guid = $1", episodeGuid).Scan(&episodeId)
	if err == nil {
		// found the entry
		return episodeId, false, nil
	}

	if err.Error() != "no rows in result set" {
		// some other real error
		return "", false, err
	}

	// no existing entry, create one
	err = dbpool.QueryRow(context.Background(), "INSERT INTO episodes (guid, podcast_id, audio_url) VALUES ($1, $2, $3) RETURNING id", episodeGuid, podcastId, mediaUrl).Scan(&episodeId)
	if err != nil {
		return "", false, err
	}
	return episodeId, true, nil
}

func getOrCreateUserEpisode(dbpool *pgxpool.Pool, userId string, episodeGuid string, podcastId string, mediaUrl string) (string, bool, error) {
	var userEpisodeId string
	err := dbpool.QueryRow(context.Background(), "SELECT id from user_episodes WHERE user_id = $1 AND episode_guid = $2", userId, episodeGuid).Scan(&userEpisodeId)
	if err == nil {
		// found the entry
		return userEpisodeId, false, nil
	}

	if err.Error() != "no rows in result set" {
		// some other real error
		return "", false, err
	}

	// get or create episode
	episodeId, _, err := getOrCreateEpisode(dbpool, episodeGuid, podcastId, mediaUrl)
	if err != nil {
		return "", false, err
	}

	// no existing entry, create one
	err = dbpool.QueryRow(context.Background(), "INSERT INTO user_episodes (user_id, episode_guid, episode_id) VALUES ($1, $2, $3) RETURNING id", userId, episodeGuid, episodeId).Scan(&userEpisodeId)
	if err != nil {
		return "", false, err
	}
	return userEpisodeId, true, nil
}
