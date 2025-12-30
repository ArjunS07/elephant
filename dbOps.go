package main

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func getOrCreateEpisode(dbpool *pgxpool.Pool, episodeGuid string, podcastId string, mediaUrl string) (string, bool, error) {
	var episodeId string
	err := dbpool.QueryRow(context.Background(), "SELECT id from episodes WHERE guid = $1", episodeGuid).Scan(&episodeId)
	if err == nil {
		// found the entry
		return episodeId, false, nil
	}

	if err.Error() != "no rows in result set" {
		// some other real error
		return "", false, err
	}

	// no existing entry, create one
	err = dbpool.QueryRow(context.Background(), "INSERT INTO episodes (guid, podcast_id, audio_url) VALUES ($1, $2, $3) RETURNING id", episodeGuid, podcastId, mediaUrl).Scan(&episodeId)
	if err != nil {
		return "", false, err
	}
	return episodeId, true, nil
}

func getOrCreateUserEpisode(dbpool *pgxpool.Pool, userId string, episodeGuid string, podcastId string, mediaUrl string) (string, bool, error) {
	var userEpisodeId string
	err := dbpool.QueryRow(context.Background(), "SELECT id from user_episodes WHERE user_id = $1 AND episode_guid = $2", userId, episodeGuid).Scan(&userEpisodeId)
	if err == nil {
		// found the entry
		return userEpisodeId, false, nil
	}

	if err.Error() != "no rows in result set" {
		// some other real error
		return "", false, err
	}

	// get or create episode
	episodeId, _, err := getOrCreateEpisode(dbpool, episodeGuid, podcastId, mediaUrl)
	if err != nil {
		return "", false, err
	}

	// no existing entry, create one
	err = dbpool.QueryRow(context.Background(), "INSERT INTO user_episodes (user_id, episode_guid, episode_id) VALUES ($1, $2, $3) RETURNING id", userId, episodeGuid, episodeId).Scan(&userEpisodeId)
	if err != nil {
		return "", false, err
	}
	return userEpisodeId, true, nil
}

func getOrCreateShow(dbpool *pgxpool.Pool, feedUrl string) (string, bool, error) {
	cleanedUrl := cleanUpUrl(feedUrl)

	var showId string
	err := dbpool.QueryRow(context.Background(), "SELECT id FROM podcast_shows WHERE feed_url = $1", cleanedUrl).Scan(&showId)
	if err == nil {
		// found the entry
		return showId, false, nil
	}

	if err.Error() != "no rows in result set" {
		// some other real error
		return "", false, err
	}

	// no existing entry, create one
	err = dbpool.QueryRow(context.Background(), "INSERT INTO podcast_shows (feed_url) VALUES ($1) RETURNING id", feedUrl).Scan(&showId)
	if err != nil {
		return "", false, err
	}
	return showId, true, nil
}

func getOrCreateUserPodcast(dbpool *pgxpool.Pool, userId string, showId string, showFeedUrl string) (string, bool, error) {
	var friendlyUniqueSlug string
	err := dbpool.QueryRow(context.Background(), "SELECT friendly_unique_slug FROM user_podcast_shows WHERE user_id = $1 AND podcast_show_id = $2", userId, showId).Scan(&friendlyUniqueSlug)
	if err == nil {
		// found the entry
		return friendlyUniqueSlug, false, nil
	}

	if err.Error() != "no rows in result set" {
		// some other real error
		return "", false, err
	}

	// no existing entry, create one
	friendlyUniqueSlug = genFriendlyUniqueSlug(userId, showId, showFeedUrl)
	err = dbpool.QueryRow(context.Background(), "INSERT INTO user_podcast_shows (user_id, podcast_show_id, friendly_unique_slug) VALUES ($1, $2, $3) RETURNING friendly_unique_slug", userId, showId, friendlyUniqueSlug).Scan(&friendlyUniqueSlug)
	if err != nil {
		return "", false, err
	}
	return friendlyUniqueSlug, true, nil
}
