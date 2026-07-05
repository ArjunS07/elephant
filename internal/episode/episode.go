// Package episode serves GET /api/episodes/{id}/transcript: an episode's full
// transcript for the episode page, scoped to the calling user's subscriptions.
package episode

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"elephant/internal/auth"
	"elephant/internal/store"
)

type Handler struct {
	store *store.Store
}

func New(st *store.Store) *Handler {
	return &Handler{store: st}
}

type segment struct {
	Idx     int    `json:"idx"`
	StartMs int    `json:"start_ms"`
	EndMs   int    `json:"end_ms"`
	Text    string `json:"text"`
}

type transcriptResponse struct {
	EpisodeID   string     `json:"episode_id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	PubDate     *time.Time `json:"pub_date"`
	DurationMs  int        `json:"duration_ms"`
	Segments    []segment  `json:"segments"`
}

func (h *Handler) Transcript(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserID(r)
	if userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	episodeID := chi.URLParam(r, "id")

	// Scope check doubles as existence check: not subscribed / unknown -> 404.
	title, description, pubDate, ok, err := h.store.EpisodeMeta(userID, episodeID)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "episode not found", http.StatusNotFound)
		return
	}

	segs, err := h.store.TranscriptSegments(episodeID)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	resp := transcriptResponse{
		EpisodeID:   episodeID,
		Title:       title,
		Description: description,
		PubDate:     pubDate,
		Segments:    make([]segment, len(segs)),
	}
	for i, s := range segs {
		resp.Segments[i] = segment{Idx: s.Idx, StartMs: s.StartMs, EndMs: s.EndMs, Text: s.Text}
		resp.DurationMs = s.EndMs // last segment's end is the episode length
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
