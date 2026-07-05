// Package media proxies episode audio through our server so we can record
// listening activity (and, later, trigger transcription). The upstream URL is
// never supplied by the caller. It is looked up from the episode row named by
// the encrypted token, which closes the SSRF/open-proxy hole.
package media

import (
	"io"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"elephant/internal/store"
	"elephant/internal/token"
)

// ASSUMED_BITRATE_KBPS is used to estimate seconds-played from bytes streamed.
const ASSUMED_BITRATE_KBPS = 128

// transcriptionThresholdSeconds is how much cumulative playback must stream
// before an episode is queued for transcription. It filters out HEAD probes and
// trivial scrubs. Only genuine engagement crosses it.
const transcriptionThresholdSeconds = 60

type Handler struct {
	store  *store.Store
	tokens *token.Service
}

func New(st *store.Store, tokens *token.Service) *Handler {
	return &Handler{store: st, tokens: tokens}
}

// byteCounter wraps a ResponseWriter and tallies bytes written, so we can
// estimate playback time from the volume streamed.
type byteCounter struct {
	http.ResponseWriter
	count int64
}

func (bc *byteCounter) Write(p []byte) (int, error) {
	n, err := bc.ResponseWriter.Write(p)
	bc.count += int64(n)
	return n, err
}

func copyHeader(src http.Header, dest http.Header, key string) {
	if val := src.Get(key); val != "" {
		dest.Set(key, val)
	}
}

// handleHeadRequest answers a HEAD by forwarding the upstream's size/type.
// Podcast apps use HEAD to learn the file size without downloading it.
func handleHeadRequest(w http.ResponseWriter, r *http.Request, targetURL string) {
	req, _ := http.NewRequestWithContext(r.Context(), "HEAD", targetURL, nil)
	client := &http.Client{Timeout: 5 * time.Second}

	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, "Upstream unreachable", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Length", resp.Header.Get("Content-Length"))
	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.Header().Set("Accept-Ranges", resp.Header.Get("Accept-Ranges"))
	w.WriteHeader(resp.StatusCode)
}

func (h *Handler) Stream(w http.ResponseWriter, r *http.Request) {
	tok := chi.URLParam(r, "token")

	// The token authorizes the user AND names the episode; the audio URL is
	// not a caller parameter, so we look it up from the episode row.
	userID, episodeID, err := h.tokens.Parse(tok)
	if err != nil {
		http.Error(w, "Invalid token", http.StatusBadRequest)
		return
	}

	audioURL, err := h.store.EpisodeAudioURL(episodeID)
	if err != nil {
		http.Error(w, "Episode not found", http.StatusNotFound)
		return
	}

	// Record that this user is accessing this episode.
	userEpisodeID, _, err := h.store.GetOrCreateUserEpisode(userID, episodeID)
	if err != nil {
		log.Printf("Error getting/creating user episode: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	log.Printf("User %s accessing episode %s (user_episode_id: %s)", userID, episodeID, userEpisodeID)

	// Podcast apps issue HEAD to check file size without downloading.
	if r.Method == "HEAD" {
		handleHeadRequest(w, r, audioURL)
		return
	}

	client := &http.Client{Timeout: 0}

	req, err := http.NewRequestWithContext(r.Context(), "GET", audioURL, nil)
	if err != nil {
		http.Error(w, "Failed to create request", http.StatusInternalServerError)
		return
	}

	// Forward the headers the upstream needs for range requests and analytics.
	copyHeader(r.Header, req.Header, "Range")
	copyHeader(r.Header, req.Header, "User-Agent")
	if clientIP := r.Header.Get("X-Forwarded-For"); clientIP != "" {
		req.Header.Set("X-Forwarded-For", clientIP+", "+r.RemoteAddr)
	} else {
		req.Header.Set("X-Forwarded-For", r.RemoteAddr)
	}

	resp, err := client.Do(req)
	if err != nil {
		// Suppress logs for client disconnects.
		if r.Context().Err() != nil {
			return
		}
		log.Printf("Proxy error: %v", err)
		http.Error(w, "Upstream error", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	headersToForward := []string{
		"Content-Type",
		"Content-Length",
		"Content-Range",
		"Accept-Ranges",
		"Last-Modified",
		"ETag",
		"Cache-Control",
	}
	for _, name := range headersToForward {
		if v := resp.Header.Get(name); v != "" {
			w.Header().Set(name, v)
		}
	}

	w.WriteHeader(resp.StatusCode)

	// Stream, counting bytes. Even on a mid-stream disconnect we record what
	// was sent, so partial listens still count.
	counter := &byteCounter{ResponseWriter: w}
	_, err = io.Copy(counter, resp.Body)

	if counter.count > 0 {
		bitrate := ASSUMED_BITRATE_KBPS * 1000 // bits per second
		secondsPlayed := (counter.count * 8) / int64(bitrate)
		totalSeconds, dbErr := h.store.AddPlaybackSeconds(userEpisodeID, secondsPlayed)
		if dbErr != nil {
			log.Printf("Failed to update progress: %v", dbErr)
		} else if totalSeconds >= transcriptionThresholdSeconds {
			// Enough has actually streamed to count as engagement — queue it.
			// Idempotent, so re-crossing on later requests is harmless.
			if err := h.store.EnqueueTranscription(episodeID); err != nil {
				log.Printf("Failed to enqueue transcription: %v", err)
			}
		}
	}
}
