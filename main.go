package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	tokenAuth = jwtauth.New("HS256", []byte("your-secret-key"), nil)

	// routes
	r := chi.NewRouter()
	r.Post("/api/register", registerHandler)
	r.Post("/api/login", loginHandler)
	r.Group(func(r chi.Router) {
		r.Use(jwtauth.Verifier(tokenAuth))
		r.Use(jwtauth.Authenticator(tokenAuth))
		r.Post("/api/podcasts/register", registerPodcastHandler)
	})

	r.Get("/feeds/{userId}/{urlHash}/", podcastFeedHandler)
	r.Get("/media/{userId}/{mediaPath:.*}", mediaStreamingProxyHandler)
	
    log.Println("Server starting on :8080")
    log.Fatal(http.ListenAndServe(":8080", r))
}

func registerPodcastHandler(w http.ResponseWriter, r *http.Request) {
	_, claims, _ := jwtauth.FromContext(r.Context())
    userID := claims["user_id"]
	if userID == nil {
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
	if err != nil || resp.StatusCode != http.StatusOK{
		if resp != nil {
			fmt.Printf("StatusCode: %d\n", resp.StatusCode)
		}
		fmt.Printf("Error fetching feed: %v\n", err)
		http.Error(w, "failed to fetch podcast feed from provided URL", http.StatusBadRequest)
		return
	}

    id, created, err := getOrCreatePodcast(dbpool, feedURL)
    if err != nil {
        log.Printf("Error: %v", err)
        http.Error(w, "Internal server error", http.StatusInternalServerError)
        return
    }

	url := getUserPodcastUrl(fmt.Sprintf("%v", userID), id)
    if created {
		fmt.Fprintf(w, `{"message": "Created podcast", "url": "%s"}`, url)
    } else {
		fmt.Fprintf(w, `{"message": "Podcast already registered", "url": "%s"}`, url)
    }
}

func getUserPodcastUrl(userId string, podcastId string) string {
	return fmt.Sprintf("http://localhost:8080/feeds/%s/%s/", userId, podcastId)
}

func getOrCreatePodcast(dbpool *pgxpool.Pool, feedUrl string) (string, bool, error){
	// semi-unique hash generator
	cleanedUrl := cleanUpUrl(feedUrl)
	hashBytes := sha256.Sum256([]byte(cleanedUrl))
	hashString := hex.EncodeToString(hashBytes[:8]) // 16 chars

	sliceRange := min(50, len(cleanedUrl))
	slugged := slug.Make(cleanedUrl[:sliceRange])
	hashString = slugged + "-" + hashString
	log.Printf("Generated hashString: %s", hashString)

	var dbHash string; // should be the same
	err := dbpool.QueryRow(context.Background(), "SELECT url_hash from podcasts WHERE url_hash = $1", hashString).Scan(&dbHash)
	if (err == nil){
		// found the entry
		return dbHash, false, nil
	}

	if (err.Error() != "no rows in result set"){
		// some other real error
		return "", false, err
	}

	// no existing entry, create one

	err = dbpool.QueryRow(context.Background(), "INSERT INTO podcasts (feed_url, url_hash) VALUES ($1, $2) RETURNING url_hash", feedUrl, hashString).Scan(&dbHash)
	if (err != nil){
		return "", false, err
	}
	return dbHash, true, nil
}

func podcastFeedHandler(w http.ResponseWriter, r *http.Request){
	userId := chi.URLParam(r, "userId")
	urlHash := chi.URLParam(r, "urlHash")
	log.Printf("UserID: %s, PodcastID: %s", userId, urlHash)

	cacheId := userId + ":" + urlHash

	// Try cache first
    if cached, found := feedCache.Get(cacheId); found {
        w.Header().Set("Content-Type", "application/xml; charset=utf-8")
        w.Header().Set("Cache-Control", "public, max-age=1800")
        w.Write(cached.([]byte))
        return
    }
	
	var feedUrl string
	err := dbpool.QueryRow(context.Background(), "SELECT feed_url from podcasts WHERE url_hash = $1", urlHash).Scan(&feedUrl)
	if err != nil{
		http.Error(w, "podcast not found", http.StatusNotFound)
		return
	}

	// Fetch original feed
	log.Printf("Fetching feed from URL: %s", feedUrl)
	resp, err := http.Get(feedUrl)
	if err != nil || resp.StatusCode != http.StatusOK{
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

	modifiedFeed := rewriteEnclosureURLs(string(feedContent), userId)

	feedCache.Set(cacheId, []byte(modifiedFeed), cache.DefaultExpiration)
	
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=1800")
	w.Write([]byte(modifiedFeed))
}

// rewriteEnclosureURLs finds all <enclosure url="..."> tags and rewrites the URLs
func rewriteEnclosureURLs(feedXML string, userId string) string {
	// Regex to match <enclosure url="..." ...> or <enclosure ... url="..." ...>
	// This captures the URL value in group 1
	re := regexp.MustCompile(`<enclosure\s+([^>]*?\s+)?url="([^"]+)"([^>]*?)>`)
	
	result := re.ReplaceAllStringFunc(feedXML, func(match string) string {
		// Extract the URL from the match
		urlRe := regexp.MustCompile(`url="([^"]+)"`)
		urlMatch := urlRe.FindStringSubmatch(match)
		
		if len(urlMatch) < 2 {
			return match // No URL found, return unchanged
		}
		
		originalURL := urlMatch[1]
		
		// Create proxy URL
		encodedURL := url.QueryEscape(originalURL)
		proxyURL := fmt.Sprintf("http://localhost:8080/media/%s/%s", userId, encodedURL)
		
		log.Printf("Rewriting: %s -> %s", originalURL, proxyURL)
		
		// Replace the URL in the original match
		return strings.Replace(match, originalURL, proxyURL, 1)
	})
	
	return result
}


func mediaStreamingProxyHandler(w http.ResponseWriter, r *http.Request) {
	userId := chi.URLParam(r, "userId")
	encodedMediaPath := chi.URLParam(r, "mediaPath")

	// Decode the original media URL
	originalURL, err := url.QueryUnescape(encodedMediaPath)
	if err != nil {
		log.Printf("Failed to decode media path: %v", err)
		http.Error(w, "Invalid media URL", http.StatusBadRequest)
		return
	}

	log.Printf("Media request - UserID: %s, Original URL: %s", userId, originalURL)

	client := &http.Client{
		// No timeout for streaming, or use a very long one
		Timeout: 0, 
	}

	// FIX: Use NewRequestWithContext to handle client disconnects
	req, err := http.NewRequestWithContext(r.Context(), "GET", originalURL, nil)
	if err != nil {
		log.Printf("Failed to create request: %v", err)
		http.Error(w, "Failed to fetch media", http.StatusInternalServerError)
		return
	}

	// Forward Range header if present
	if rangeHeader := r.Header.Get("Range"); rangeHeader != "" {
		req.Header.Set("Range", rangeHeader)
		log.Printf("Forwarding Range header: %s", rangeHeader)
	}

	// Forward User-Agent
	if ua := r.Header.Get("User-Agent"); ua != "" {
		req.Header.Set("User-Agent", ua)
	}

	resp, err := client.Do(req)
	if err != nil {
		// Handle context cancellation specifically to avoid log noise
		if r.Context().Err() != nil {
			log.Printf("Client disconnected user %s", userId)
			return
		}
		log.Printf("Failed to fetch media: %v", err)
		http.Error(w, "Failed to fetch media", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// FIX: Careful Header Copying
	// Do not copy hop-by-hop headers or Content-Encoding/Length if body is modified/decoded
	hopByHop := map[string]bool{
		"Connection": true,
		"Keep-Alive": true,
		"Proxy-Authenticate": true,
		"Proxy-Authorization": true,
		"Te": true,
		"Trailers": true,
		"Transfer-Encoding": true,
		"Upgrade": true,
	}

	for key, values := range resp.Header {
		if hopByHop[key] {
			continue
		}
		// Go's http.Client automatically decompresses GZIP.
		// If we forward "Content-Encoding: gzip", the client expects compressed data
		// but receives raw data, causing playback failure.
		if key == "Content-Encoding" {
			continue
		}
		// Content-Length might be wrong if we decoded the body.
		// It's safer to let the Transfer-Encoding: chunked handle it, 
		// unless we are sure we are sending the exact bytes.
		// Since Go decompresses, size changes. Drop it.
		if key == "Content-Length" && resp.Uncompressed {
			continue
		}

		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	w.WriteHeader(resp.StatusCode)

	written, err := io.Copy(w, resp.Body)
	if err != nil {
		log.Printf("Error streaming media: %v (wrote %d bytes)", err, written)
		return
	}

	log.Printf("Successfully streamed %d bytes to user %s", written, userId)
}