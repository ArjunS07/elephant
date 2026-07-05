package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/go-chi/jwtauth/v5"
	"github.com/gosimple/slug"
)

func cleanUpUrl(feedUrl string) string {
	// strip https:// or http:// from the URL for normalization
	cleanUrl := feedUrl
	if len(feedUrl) >= 7 && feedUrl[:7] == "http://" {
		cleanUrl = feedUrl[7:]
	}
	if len(feedUrl) >= 8 && feedUrl[:8] == "https://" {
		cleanUrl = feedUrl[8:]
	}

	// get rid of www
	if len(cleanUrl) >= 4 && cleanUrl[:4] == "www." {
		cleanUrl = cleanUrl[4:]
	}

	return cleanUrl
}

func getPublicHost() string {
	publicHost := os.Getenv("PUBLIC_HOST")
	if publicHost == "" {
		publicHost = "http://localhost:8080"
	}
	return publicHost
}

func genFriendlyUniqueSlug(userId string, showId string, showUrl string) string {
	// cap to 32 characters
	feedUrlSlug := slug.Make(cleanUpUrl(showUrl))
	feedUrlSlug = feedUrlSlug[:min(32, len(feedUrlSlug))]

	// take 32 characters of hash
	hashBytes := sha256.Sum256([]byte(userId + showId))
	hashSuffix := fmt.Sprintf("%x", hashBytes)[:8]

	return fmt.Sprintf("%s-%s", feedUrlSlug, hashSuffix)
}

func getUserShowFeedUrl(slug string) string {
	return fmt.Sprintf("%s/feeds/%s", getPublicHost(), slug)
}

func getUserId(r *http.Request) string {
	_, claims, _ := jwtauth.FromContext(r.Context())
	userID := claims["user_id"]
	if userID == nil {
		return ""
	}
	return userID.(string)
}

func encryptPlainBytestringToAes(bytestring []byte, encryptionKey string) (encoded []byte, err error) {
	key := []byte(encryptionKey)
	if len(key) != 32 {
		return nil, errors.New("encryption key must be 32 bytes")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}

	encrypted := gcm.Seal(nonce, nonce, bytestring, nil)
	return encrypted, nil
}

func decryptAes256ToPlainByteString(token string, encryptionKey string) (decoded []byte, err error) {
	key := []byte(encryptionKey)
	ciphertext, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, errors.New("invalid token")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}
	return plaintext, nil
}
