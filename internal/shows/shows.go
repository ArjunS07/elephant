// Package shows serves the user's podcast list and per-show listening history:
// GET /api/shows and GET /api/shows/{id}, both scoped to the calling user.
package shows

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"elephant/internal/auth"
	"elephant/internal/store"
)

type Handler struct {
	store      *store.Store
	publicHost string
}

func New(st *store.Store, publicHost string) *Handler {
	return &Handler{store: st, publicHost: publicHost}
}

func (h *Handler) customFeedURL(slug string) string {
	return fmt.Sprintf("%s/feeds/%s", h.publicHost, slug)
}

type showSummary struct {
	ShowID        string    `json:"show_id"`
	Title         string    `json:"title"`
	Description    string    `json:"description"`
	ImageURL      string    `json:"image_url"`
	CustomFeedURL string    `json:"custom_feed_url"`
	SourceFeedURL string    `json:"source_feed_url"`
	SubscribedAt  time.Time `json:"subscribed_at"`
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserID(r)
	if userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	shows, err := h.store.ListUserShows(userID)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	out := make([]showSummary, len(shows))
	for i, s := range shows {
		out[i] = showSummary{
			ShowID:        s.ShowID,
			Title:         s.Title,
			Description:   s.Description,
			ImageURL:      s.ImageURL,
			CustomFeedURL: h.customFeedURL(s.Slug),
			SourceFeedURL: s.SourceFeedURL,
			SubscribedAt:  s.SubscribedAt,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

type episodeEntry struct {
	EpisodeID          string     `json:"episode_id"`
	Title              string     `json:"title"`
	PubDate            *time.Time `json:"pub_date"`
	TranscriptStatus   string     `json:"transcript_status"`
	FirstPlayedAt      *time.Time `json:"first_played_at"`
	LastPlayedAt       *time.Time `json:"last_played_at"`
	TotalSecondsPlayed int        `json:"total_seconds_played"`
}

type showDetailResponse struct {
	Show     showSummary    `json:"show"`
	Episodes []episodeEntry `json:"episodes"`
}

func (h *Handler) Detail(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserID(r)
	if userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	showID := chi.URLParam(r, "id")

	// Scope check doubles as existence check: not subscribed / unknown -> 404.
	show, ok, err := h.store.UserShowByID(userID, showID)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "show not found", http.StatusNotFound)
		return
	}

	episodes, err := h.store.ShowEpisodes(userID, showID)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	resp := showDetailResponse{
		Show: showSummary{
			ShowID:        show.ShowID,
			Title:         show.Title,
			Description:   show.Description,
			ImageURL:      show.ImageURL,
			CustomFeedURL: h.customFeedURL(show.Slug),
			SourceFeedURL: show.SourceFeedURL,
			SubscribedAt:  show.SubscribedAt,
		},
		Episodes: make([]episodeEntry, len(episodes)),
	}
	for i, e := range episodes {
		resp.Episodes[i] = episodeEntry{
			EpisodeID:          e.EpisodeID,
			Title:              e.Title,
			PubDate:            e.PubDate,
			TranscriptStatus:   e.TranscriptStatus,
			FirstPlayedAt:      e.FirstPlayedAt,
			LastPlayedAt:       e.LastPlayedAt,
			TotalSecondsPlayed: e.TotalSecondsPlayed,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
