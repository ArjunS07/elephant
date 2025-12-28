package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

var dbpool *pgxpool.Pool

func main() {
	// setup
    var err error
	
	err = godotenv.Load()
    if err != nil {
        log.Println("No .env file found or error loading .env file")
    }

    dbpool, err = pgxpool.New(context.Background(), os.Getenv(("DATABASE_URL")));
    if err != nil {
        log.Fatalf("Unable to create pool: %v\n", err)
    }
    defer dbpool.Close()

	// routes
    http.HandleFunc("/api/podcasts/register", registerPodcastHandler)
    
    log.Println("Server starting on :8080")
    log.Fatal(http.ListenAndServe(":8080", nil))
}


func registerPodcastHandler(w http.ResponseWriter, r *http.Request) {

	feedUrl := r.URL.Query().Get("feed_url");
	if (feedUrl == ""){
		http.Error(w, "feed_url parameter is required", http.StatusBadRequest);
		return;
	}

	id, created, err := getOrCreatePodcast(dbpool, feedUrl);
	if (err != nil){
		fmt.Printf("%s", err.Error());
		http.Error(w, "internal server error", http.StatusInternalServerError);
		return;
	}
	
	if (created){
		fmt.Fprintf(w, "Podcast registered with ID: %s\n", id);
	} else {
		fmt.Fprintf(w, "Podcast already exists with ID: %s\n", id);
	}
}

func getOrCreatePodcast(dbpool *pgxpool.Pool, feedUrl string) (string, bool, error){
	// semi-unique hash generator
	cleanedUrl := cleanUpUrl(feedUrl);
	hashBytes := sha256.Sum256([]byte(cleanedUrl));
	hashString := hex.EncodeToString(hashBytes[:]);

	var id string;
	err := dbpool.QueryRow(context.Background(), "SELECT id from podcasts WHERE url_hash = $1", hashString).Scan(&id);
	if (err == nil){
		// found the entry
		return id, false, nil
	}

	if (err.Error() != "no rows in result set"){
		// some other real error
		return "", false, err
	}

	// no existing entry, create one
	err = dbpool.QueryRow(context.Background(), "INSERT INTO podcasts (feed_url, url_hash) VALUES ($1, $2) RETURNING id", feedUrl, hashString).Scan(&id);
	if (err != nil){
		return "", false, err
	}
	return id, true, nil
}

func mediaStreamingProxyHandler(w http.ResponseWriter, r *http.Request){
}