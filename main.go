package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/jwtauth/v5"
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

	r.Get("/feeds/{userId}/{podcastId}/", podcastFeedHandler)
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
	hashString := hex.EncodeToString(hashBytes[:])

	var id string
	err := dbpool.QueryRow(context.Background(), "SELECT id from podcasts WHERE url_hash = $1", hashString).Scan(&id)
	if (err == nil){
		// found the entry
		return id, false, nil
	}

	if (err.Error() != "no rows in result set"){
		// some other real error
		return "", false, err
	}

	// no existing entry, create one
	err = dbpool.QueryRow(context.Background(), "INSERT INTO podcasts (feed_url, url_hash) VALUES ($1, $2) RETURNING id", feedUrl, hashString).Scan(&id)
	if (err != nil){
		return "", false, err
	}
	return id, true, nil
}

func podcastFeedHandler(w http.ResponseWriter, r *http.Request){
	userId := chi.URLParam(r, "userId")
	podcastId := chi.URLParam(r, "podcastId")
	log.Printf("UserID: %s, PodcastID: %s", userId, podcastId)

	cacheId := userId + ":" + podcastId

	// Try cache first
    if cached, found := feedCache.Get(cacheId); found {
        w.Header().Set("Content-Type", "application/xml; charset=utf-8")
        w.Header().Set("Cache-Control", "public, max-age=1800")
        w.Write(cached.([]byte))
        return
    }
	
	var feedUrl string
	err := dbpool.QueryRow(context.Background(), "SELECT feed_url from podcasts WHERE id = $1", podcastId).Scan(&feedUrl)
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

	// Read and parse feed
	feedContent, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "failed to read feed", http.StatusInternalServerError)
		return
	}

	// Parse XML
	var rss RSS
	err = xml.Unmarshal(feedContent, &rss)
	if err != nil {
		log.Printf("Failed to parse XML: %v", err)
		// If parsing fails, just return original feed
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		w.Write(feedContent)
		return
	}

	// Rewrite media URLs in each item
	for i := range rss.Channel.Items {
		originalURL := rss.Channel.Items[i].Enclosure.URL
		if originalURL != "" {
			// URL encode the original media URL
			encodedURL := url.QueryEscape(originalURL)
			// Create our proxy URL
			proxyURL := fmt.Sprintf("http://localhost:8080/media/%s/%s", userId, encodedURL)
			rss.Channel.Items[i].Enclosure.URL = proxyURL
			
			log.Printf("Rewrote URL: %s -> %s", originalURL, proxyURL)
		}
	}

	// pretty print back to XML
	modifiedFeed, err := xml.MarshalIndent(rss, "", "  ")
	if err != nil {
		log.Printf("Failed to marshal XML: %v", err)
		http.Error(w, "failed to generate feed", http.StatusInternalServerError)
		return
	}

	xmlHeader := []byte(xml.Header)
	finalFeed := append(xmlHeader, modifiedFeed...)
	feedCache.Set(cacheId, finalFeed, cache.DefaultExpiration)
	
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=1800")
	w.Write(finalFeed)
}


func mediaStreamingProxyHandler(w http.ResponseWriter, r *http.Request){
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

	// Create request to original URL
	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	
	req, err := http.NewRequest("GET", originalURL, nil)
	if err != nil {
		log.Printf("Failed to create request: %v", err)
		http.Error(w, "Failed to fetch media", http.StatusInternalServerError)
		return
	}

	// Forward Range header if present (for seeking in podcast apps)
	if rangeHeader := r.Header.Get("Range"); rangeHeader != "" {
		req.Header.Set("Range", rangeHeader)
		log.Printf("Forwarding Range header: %s", rangeHeader)
	}

	// Forward User-Agent
	if ua := r.Header.Get("User-Agent"); ua != "" {
		req.Header.Set("User-Agent", ua)
	}

	// Fetch from origin
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Failed to fetch media: %v", err)
		http.Error(w, "Failed to fetch media", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Copy response headers
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	// should be 206 Partial Content
	w.WriteHeader(resp.StatusCode)

	// Stream the content
	// io.Copy handles chunked transfer efficiently
	written, err := io.Copy(w, resp.Body)
	if err != nil {
		// Don't send error response here - headers already sent
		log.Printf("Error streaming media: %v (wrote %d bytes)", err, written)
		return
	}

	log.Printf("Successfully streamed %d bytes to user %s", written, userId)
	
	// TODO: After successful stream, you could:
	// - Register this episode for transcription if >30 seconds played
	// - Update user's listening history
	// - Track position for resume functionality
}
