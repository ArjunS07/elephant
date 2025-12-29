package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
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

	// Read feed content
	feedContent, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "failed to read feed", http.StatusInternalServerError)
		return
	}

	// TODO: Parse and rewrite media URLs here
	feedCache.Set(cacheId, feedContent, cache.DefaultExpiration)
	
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=1800") // Cache for 0.5 hrs	
	w.Write(feedContent)
}


func mediaStreamingProxyHandler(w http.ResponseWriter, r *http.Request){

}