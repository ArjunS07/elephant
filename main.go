package main

import (
	"context"
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
