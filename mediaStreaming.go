package main

import (
	"net/http"
	"time"
)

const ASSUMED_BITRATE_KBPS = 128

type byteCounter struct {
	http.ResponseWriter
	count int64
}

func (bc *byteCounter) Write(p []byte) (int, error) {
	n, err := bc.ResponseWriter.Write(p)
	bc.count += int64(n)
	return n, err
}

func handleHeadRequest(w http.ResponseWriter, r *http.Request, targetURL string) {
	req, _ := http.NewRequestWithContext(r.Context(), "HEAD", targetURL, nil)
	client := &http.Client{Timeout: 5 * time.Second}

	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, "Upstream unreachable", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Forward size and type
	w.Header().Set("Content-Length", resp.Header.Get("Content-Length"))
	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.Header().Set("Accept-Ranges", resp.Header.Get("Accept-Ranges"))
	w.WriteHeader(resp.StatusCode)
}

func copyHeader(src http.Header, dest http.Header, key string) {
	if val := src.Get(key); val != "" {
		dest.Set(key, val)
	}
}
