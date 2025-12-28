package main

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