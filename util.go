package main

import (
	"crypto/sha256"
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
