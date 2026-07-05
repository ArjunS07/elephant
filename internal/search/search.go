// Package search serves POST /api/search: hybrid retrieval (semantic KNN fused
// with lexical BM25 via RRF, then cross-encoder reranked) over the calling user's
// subscribed shows. Results are grouped by episode and paginated. Go owns the
// fusion; the model box does embedding + reranking.
package search

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"

	"elephant/internal/auth"
	"elephant/internal/modelbox"
	"elephant/internal/store"
)

const (
	candidateN      = 100 // rows to pull from each of the two searches
	rerankPool      = 100 // fused candidates to send to the reranker
	perEpisodeCap   = 10  // matching chunks kept per episode
	defaultPageSize = 20  // episodes per page
	maxPageSize     = 50
	rrfK            = 60 // RRF damping constant
)

type Handler struct {
	store    *store.Store
	modelbox *modelbox.Client
}

func New(st *store.Store, mb *modelbox.Client) *Handler {
	return &Handler{store: st, modelbox: mb}
}

type searchRequest struct {
	Query    string `json:"query"`
	Page     int    `json:"page"`      // 1-based; defaults to 1
	PageSize int    `json:"page_size"` // defaults to defaultPageSize
}

type chunkResult struct {
	Idx     int     `json:"idx"`
	StartMs int     `json:"start_ms"`
	EndMs   int     `json:"end_ms"`
	Text    string  `json:"text"`
	Score   float64 `json:"score"`
}

type episodeResult struct {
	EpisodeID  string        `json:"episode_id"`
	Title      string        `json:"title"`
	DurationMs int           `json:"duration_ms"`
	Score      float64       `json:"score"` // best chunk score
	Chunks     []chunkResult `json:"chunks"`
}

type searchResponse struct {
	Page          int             `json:"page"`
	PageSize      int             `json:"page_size"`
	TotalEpisodes int             `json:"total_episodes"`
	HasMore       bool            `json:"has_more"`
	Results       []episodeResult `json:"results"`
}

func (h *Handler) Search(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserID(r)
	if userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req searchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Query == "" {
		http.Error(w, "query required", http.StatusBadRequest)
		return
	}
	page, pageSize := paginate(req.Page, req.PageSize)

	ctx := r.Context()
	vecs, err := h.modelbox.Embed(ctx, []string{req.Query}, true)
	if err != nil || len(vecs) == 0 {
		http.Error(w, "embedding failed", http.StatusBadGateway)
		return
	}

	semantic, err := h.store.SemanticSearch(userID, vecs[0], candidateN)
	if err != nil {
		http.Error(w, "search failed", http.StatusInternalServerError)
		return
	}
	lexical, err := h.store.LexicalSearch(userID, req.Query, candidateN)
	if err != nil {
		http.Error(w, "search failed", http.StatusInternalServerError)
		return
	}

	// Fuse, rerank the pool once, then group into episodes.
	pool := rrf(semantic, lexical)
	if len(pool) > rerankPool {
		pool = pool[:rerankPool]
	}
	scored, err := h.rerankScores(ctx, req.Query, pool)
	if err != nil {
		http.Error(w, "rerank failed", http.StatusBadGateway)
		return
	}
	episodes := groupByEpisode(pool, scored)

	// Paginate episodes, then attach durations for just the page.
	total := len(episodes)
	offset := (page - 1) * pageSize
	if offset > total {
		offset = total
	}
	end := offset + pageSize
	if end > total {
		end = total
	}
	pageEpisodes := episodes[offset:end]
	h.attachDurations(pageEpisodes)

	resp := searchResponse{
		Page:          page,
		PageSize:      pageSize,
		TotalEpisodes: total,
		HasMore:       end < total,
		Results:       pageEpisodes,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func paginate(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = defaultPageSize
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}
	return page, pageSize
}

// rrf fuses the two ranked lists by reciprocal rank: a chunk's score is the sum
// over the lists it appears in of 1/(rrfK + rank). Returns chunks best first.
func rrf(lists ...[]store.Chunk) []store.Chunk {
	type entry struct {
		chunk store.Chunk
		score float64
	}
	byKey := map[string]*entry{}
	for _, list := range lists {
		for rank, c := range list {
			key := c.EpisodeID + ":" + strconv.Itoa(c.Idx)
			e := byKey[key]
			if e == nil {
				e = &entry{chunk: c}
				byKey[key] = e
			}
			e.score += 1.0 / float64(rrfK+rank)
		}
	}

	entries := make([]*entry, 0, len(byKey))
	for _, e := range byKey {
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].score > entries[j].score })

	fused := make([]store.Chunk, len(entries))
	for i, e := range entries {
		fused[i] = e.chunk
	}
	return fused
}

// rerankScores returns the cross-encoder score for each chunk in pool, aligned by
// index.
func (h *Handler) rerankScores(ctx context.Context, query string, pool []store.Chunk) ([]float64, error) {
	if len(pool) == 0 {
		return nil, nil
	}
	passages := make([]string, len(pool))
	for i, c := range pool {
		passages[i] = c.Text
	}
	scores, err := h.modelbox.Rerank(ctx, query, passages)
	if err != nil {
		return nil, err
	}
	return scores, nil
}

// groupByEpisode collapses the scored chunk pool into episodes: each episode keeps
// its top perEpisodeCap chunks by score (then sorted by start_ms for ticks), and
// episodes are ordered by their best chunk score.
func groupByEpisode(pool []store.Chunk, scores []float64) []episodeResult {
	byEpisode := map[string]*episodeResult{}
	var order []string
	for i, c := range pool {
		ep := byEpisode[c.EpisodeID]
		if ep == nil {
			ep = &episodeResult{EpisodeID: c.EpisodeID, Title: c.Title}
			byEpisode[c.EpisodeID] = ep
			order = append(order, c.EpisodeID)
		}
		ep.Chunks = append(ep.Chunks, chunkResult{
			Idx: c.Idx, StartMs: c.StartMs, EndMs: c.EndMs, Text: c.Text, Score: scores[i],
		})
	}

	episodes := make([]episodeResult, 0, len(order))
	for _, id := range order {
		ep := byEpisode[id]
		// Rank chunks by score, cap, then present in time order.
		sort.Slice(ep.Chunks, func(i, j int) bool { return ep.Chunks[i].Score > ep.Chunks[j].Score })
		if len(ep.Chunks) > perEpisodeCap {
			ep.Chunks = ep.Chunks[:perEpisodeCap]
		}
		ep.Score = ep.Chunks[0].Score
		sort.Slice(ep.Chunks, func(i, j int) bool { return ep.Chunks[i].StartMs < ep.Chunks[j].StartMs })
		episodes = append(episodes, *ep)
	}
	sort.Slice(episodes, func(i, j int) bool { return episodes[i].Score > episodes[j].Score })
	return episodes
}

// attachDurations fills DurationMs for the given episodes from the store.
func (h *Handler) attachDurations(episodes []episodeResult) {
	if len(episodes) == 0 {
		return
	}
	ids := make([]string, len(episodes))
	for i, ep := range episodes {
		ids[i] = ep.EpisodeID
	}
	durations, err := h.store.EpisodeDurations(ids)
	if err != nil {
		return // duration is non-essential; leave zero on error
	}
	for i := range episodes {
		episodes[i].DurationMs = durations[episodes[i].EpisodeID]
	}
}
