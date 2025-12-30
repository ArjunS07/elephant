package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"os"
	"strings"
)

func generateMediaToken(userId string, episodeGuid string, podcastId string) (string, error) {
	key := []byte(os.Getenv("ENCRYPTION_KEY"))

	if len(key) != 32 {
		return "", errors.New("encryption key must be 32 bytes")
	}

	plaintext := []byte(userId + "|" + episodeGuid + "|" + podcastId)

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.RawURLEncoding.EncodeToString(ciphertext), nil
}

func decryptMediaToken(token string) (userId, episodeGuid string, podcastId string, err error) {
	key := []byte(os.Getenv("ENCRYPTION_KEY"))

	ciphertext, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return "", "", "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", "", "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", "", "", err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", "", "", errors.New("invalid token")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", "", "", err
	}

	parts := strings.Split(string(plaintext), "|")
	if len(parts) != 3 {
		return "", "", "", errors.New("invalid token format")
	}

	return parts[0], parts[1], parts[2], nil
}
