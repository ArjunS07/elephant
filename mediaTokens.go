package main

import (
	"encoding/base64"
	"encoding/json"
	"os"
)

type tokenPayload struct {
	UserID      string `json:"u"`
	EpisodeGUID string `json:"e"`
	PodcastID   string `json:"p"`
}

// Generate an AES-256-GCM encrypted, base64url-encoded token
// whose plaintext corresponds to a JSON encoding of tokenPayload (userId, episodeGUID, podcastId)
func generateMediaToken(userId string, episodeGuid string, podcastId string) (string, error) {
	bytestring, _ := json.Marshal(tokenPayload{UserID: userId, EpisodeGUID: episodeGuid, PodcastID: podcastId})
	encrypted, err := encryptPlainBytestringToAes(bytestring, os.Getenv("ENCRYPTION_KEY"))
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(encrypted), nil
}

func decryptMediaToken(token string) (userId, episodeGuid string, podcastId string, err error) {
	plainbytes, err := decryptAes256ToPlainByteString(token, os.Getenv("ENCRYPTION_KEY"))
	if err != nil {
		return "", "", "", err
	}
	var decodedPayload tokenPayload
	err = json.Unmarshal(plainbytes, &decodedPayload)
	if err != nil {
		return "", "", "", err
	}
	return decodedPayload.UserID, decodedPayload.EpisodeGUID, decodedPayload.PodcastID, nil

}
