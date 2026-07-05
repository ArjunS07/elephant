// Package modelbox is the Go client for the Python model box, which serves the
// embedder and reranker over HTTP. Go owns Postgres and fusion; the model box is
// stateless model inference.
package modelbox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Client struct {
	baseURL string
	http    *http.Client
}

func New(baseURL string) *Client {
	return &Client{baseURL: baseURL, http: &http.Client{Timeout: 30 * time.Second}}
}

// Embed returns one vector per text. isQuery selects the query-side prefix the
// embedder applies (queries and passages are embedded asymmetrically).
func (c *Client) Embed(ctx context.Context, texts []string, isQuery bool) ([][]float32, error) {
	var out struct {
		Embeddings [][]float32 `json:"embeddings"`
	}
	err := c.post(ctx, "/embed", map[string]any{"texts": texts, "is_query": isQuery}, &out)
	return out.Embeddings, err
}

// Rerank scores each passage against the query with the cross-encoder; higher is
// more relevant. Order matches the passages slice.
func (c *Client) Rerank(ctx context.Context, query string, passages []string) ([]float64, error) {
	var out struct {
		Scores []float64 `json:"scores"`
	}
	err := c.post(ctx, "/rerank", map[string]any{"query": query, "passages": passages}, &out)
	return out.Scores, err
}

func (c *Client) post(ctx context.Context, path string, body, out any) error {
	buf, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("modelbox %s: status %d", path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
