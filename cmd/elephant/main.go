package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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
	"elephant/internal/shows"
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
	showsH := shows.New(st, cfg.PublicHost)

	r := chi.NewRouter()
	r.Post("/api/register", authH.Register)
	r.Post("/api/login", authH.Login)

	r.Group(func(r chi.Router) {
		r.Use(jwtauth.Verifier(tokenAuth))
		r.Use(jwtauth.Authenticator(tokenAuth))
		r.Post("/api/podcasts/register", feedH.Register)
		r.Get("/api/shows", showsH.List)
		r.Get("/api/shows/{id}", showsH.Detail)
		r.Post("/api/search", searchH.Search)
		r.Get("/api/episodes/{id}/transcript", episodeH.Transcript)
	})

	r.Get("/feeds/{userPodcastUniqueSlug}", feedH.Serve)
	r.Get("/media/{token}/stream.mp3", mediaH.Stream)
	r.Head("/media/{token}/stream.mp3", mediaH.Stream)

	// Everything else serves the built frontend.
	r.Handle("/*", spaHandler("frontend/dist"))

	log.Println("Server starting on :8080")
	log.Fatal(http.ListenAndServe(":8080", r))
}

// spaHandler serves static files from dir, falling back to index.html for paths
// that don't map to a file so client-side routes resolve.
func spaHandler(dir string) http.HandlerFunc {
	fs := http.FileServer(http.Dir(dir))
	index := filepath.Join(dir, "index.html")
	return func(w http.ResponseWriter, r *http.Request) {
		if _, err := os.Stat(filepath.Join(dir, filepath.Clean(r.URL.Path))); os.IsNotExist(err) {
			// The SPA shell: never cache it, so a new deploy is picked up at once.
			w.Header().Set("Cache-Control", "no-cache")
			http.ServeFile(w, r, index)
			return
		}
		// Vite fingerprints asset filenames, so they can be cached forever.
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		fs.ServeHTTP(w, r)
	}
}
