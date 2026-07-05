package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/jwtauth/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/patrickmn/go-cache"

	"elephant/internal/auth"
	"elephant/internal/config"
	"elephant/internal/episode"
	"elephant/internal/feed"
	"elephant/internal/media"
	"elephant/internal/modelbox"
	"elephant/internal/search"
	"elephant/internal/store"
	"elephant/internal/token"
)

func main() {
	cfg := config.Load()

	pool, err := pgxpool.New(context.Background(), cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Unable to create pool: %v\n", err)
	}
	defer pool.Close()

	// Construct dependencies and inject into handlers
	st := store.New(pool)
	tokens := token.New(cfg.EncryptionKey)
	tokenAuth := jwtauth.New("HS256", []byte(cfg.SecretKey), nil)
	feedCache := cache.New(30*time.Minute, 10*time.Minute)

	mb := modelbox.New(cfg.ModelboxURL)

	authH := auth.New(st, tokenAuth)
	feedH := feed.New(st, tokens, feedCache, cfg.PublicHost)
	mediaH := media.New(st, tokens)
	searchH := search.New(st, mb)
	episodeH := episode.New(st)

	r := chi.NewRouter()
	r.Post("/api/register", authH.Register)
	r.Post("/api/login", authH.Login)

	r.Group(func(r chi.Router) {
		r.Use(jwtauth.Verifier(tokenAuth))
		r.Use(jwtauth.Authenticator(tokenAuth))
		r.Post("/api/podcasts/register", feedH.Register)
		r.Post("/api/search", searchH.Search)
		r.Get("/api/episodes/{id}/transcript", episodeH.Transcript)
	})

	r.Get("/feeds/{userPodcastUniqueSlug}", feedH.Serve)
	r.Get("/media/{token}/stream.mp3", mediaH.Stream)
	r.Head("/media/{token}/stream.mp3", mediaH.Stream)

	log.Println("Server starting on :8080")
	log.Fatal(http.ListenAndServe(":8080", r))
}
