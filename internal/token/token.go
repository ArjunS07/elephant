// Package token mints and parses the opaque media tokens embedded in rewritten
// feed URLs. A token is the AES-256-GCM encryption of a small JSON payload,
// base64url-encoded. Because GCM is authenticated, a token cannot be forged or
// tampered with without the key.
package token

import (
	"encoding/base64"
	"encoding/json"
	"errors"
)

type Service struct {
	key []byte
}

type payload struct {
	UserID    string `json:"u"`
	EpisodeID string `json:"e"`
}

// New returns a token Service holding the given key. As in the original code,
// the key length is validated at mint time, not here.
func New(key string) *Service {
	return &Service{key: []byte(key)}
}

// Mint seals (userID, episodeID) into a URL-safe token string.
func (s *Service) Mint(userID, episodeID string) (string, error) {
	if len(s.key) != 32 {
		return "", errors.New("encryption key must be 32 bytes")
	}
	plaintext, err := json.Marshal(payload{UserID: userID, EpisodeID: episodeID})
	if err != nil {
		return "", err
	}
	ciphertext, err := encrypt(s.key, plaintext)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(ciphertext), nil
}

// Parse reverses Mint. It fails if the token was tampered with or is malformed.
func (s *Service) Parse(tok string) (userID, episodeID string, err error) {
	ciphertext, err := base64.RawURLEncoding.DecodeString(tok)
	if err != nil {
		return "", "", err
	}
	plaintext, err := decrypt(s.key, ciphertext)
	if err != nil {
		return "", "", err
	}
	var p payload
	if err := json.Unmarshal(plaintext, &p); err != nil {
		return "", "", err
	}
	return p.UserID, p.EpisodeID, nil
}
