package feed

import (
	"crypto/sha256"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gosimple/slug"
	"github.com/patrickmn/go-cache"

	"elephant/internal/auth"
	"elephant/internal/store"
	"elephant/internal/token"
)

type Handler struct {
	store        *store.Store
	tokenService *token.Service
	cache        *cache.Cache
	publicHost   string
}

func New(st *store.Store, tokenService *token.Service, c *cache.Cache, publicHost string) *Handler {
	return &Handler{store: st, tokenService: tokenService, cache: c, publicHost: publicHost}
}

func (h *Handler) userShowFeedURL(s string) string {
	return fmt.Sprintf("%s/feeds/%s", h.publicHost, s)
}

// parses the feed for its items, creates/refreshes an episode row per item,
// and swaps each <enclosure url> for a proxy URL carrying a token.
// It parses with encoding/xml but emits via string
// replacement on the raw bytes, preventing re-serialization, such that the feed stays intact.
func (h *Handler) rewriteEnclosures(feedXML []byte, userID, showID string) []byte {
	var feed rssFeed
	if err := xml.Unmarshal(feedXML, &feed); err != nil {
		log.Printf("Failed to parse feed: %v", err)
		return feedXML
	}

	if err := h.store.UpdateShowMeta(showID, feed.Channel.Title, feed.Channel.Description, channelImage(feed.Channel)); err != nil {
		log.Printf("Failed to update show metadata: %v", err)
	}

	out := string(feedXML)
	for _, it := range feed.Channel.Items {
		audioURL := it.Enclosure.URL
		if audioURL == "" {
			continue
		}

		guid, source := resolveGUID(it)
		episodeID, err := h.store.GetOrCreateEpisode(showID, guid, source, audioURL, it.Title, it.Description, parsePubDate(it.PubDate))
		if err != nil {
			log.Printf("Failed to get/create episode: %v", err)
			continue
		}

		tok, err := h.tokenService.Mint(userID, episodeID)
		if err != nil {
			log.Printf("Failed to mint token: %v", err)
			continue
		}

		// encoding/xml decodes entities when parsing (e.g. &amp; to &), but the raw
		// feed bytes we're editing still hold the encoded form. Re-escape & so the
		// search string matches what's actually in the bytes.
		rawURL := strings.ReplaceAll(audioURL, "&", "&amp;")
		proxyURL := fmt.Sprintf("%s/media/%s/stream.mp3", h.publicHost, tok)
		out = strings.Replace(out, rawURL, proxyURL, 1)
	}
	return []byte(out)
}

// parsePubDate parses an RSS date string, trying the formats feeds actually use.
// Returns nil when empty or unparseable, so the DB stores NULL.
func parsePubDate(s string) *time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	for _, layout := range []string{time.RFC1123Z, time.RFC1123, time.RFC822Z, time.RFC822} {
		if t, err := time.Parse(layout, s); err == nil {
			return &t
		}
	}
	return nil
}

// resolveGUID returns the item's publisher guid, or a synthesized one (marked
// as such) when the feed omits it.
func resolveGUID(it item) (guid, source string) {
	if g := strings.TrimSpace(it.GUID); g != "" {
		return g, "feed"
	}
	hashBytes := sha256.Sum256([]byte(it.Title + "|" + it.PubDate))
	return "synthetic:" + fmt.Sprintf("%x", hashBytes)[:16], "synthetic"
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserID(r)
	if userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	feedURL := r.URL.Query().Get("feed_url")
	if feedURL == "" {
		http.Error(w, "feed_url parameter required", http.StatusBadRequest)
		return
	}

	resp, err := http.Get(feedURL)
	if err != nil {
		http.Error(w, "failed to fetch podcast feed from provided URL", http.StatusBadRequest)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		http.Error(w, "failed to fetch podcast feed from provided URL", http.StatusBadRequest)
		return
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "failed to read podcast feed", http.StatusBadRequest)
		return
	}
	// Store the raw, fetchable URL (it is used verbatim to fetch the feed later).
	showID, _, err := h.store.GetOrCreateShow(feedURL)
	if err != nil {
		log.Printf("Error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	title, description, image := parseChannelMeta(body)
	if err := h.store.UpdateShowMeta(showID, title, description, image); err != nil {
		log.Printf("Failed to update show metadata: %v", err)
	}

	desiredSlug := genSlug(userID, showID, feedURL)
	friendlySlug, created, err := h.store.GetOrCreateUserShow(userID, showID, desiredSlug)
	if err != nil {
		log.Printf("Error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	url := h.userShowFeedURL(friendlySlug)
	w.Header().Set("Content-Type", "application/json")
	if created {
		fmt.Fprintf(w, `{"message": "Created podcast", "url": "%s"}`, url)
	} else {
		fmt.Fprintf(w, `{"message": "Podcast already registered", "url": "%s"}`, url)
	}
}

func (h *Handler) Serve(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "userPodcastUniqueSlug")

	if cached, found := h.cache.Get(slug); found {
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=1800")
		w.Write(cached.([]byte))
		return
	}

	userID, showID, err := h.store.ShowIDsBySlug(slug)
	if err != nil {
		http.Error(w, "podcast not found for this user", http.StatusNotFound)
		return
	}

	feedURL, err := h.store.FeedURLByShowID(showID)
	if err != nil {
		http.Error(w, "podcast feed not found", http.StatusNotFound)
		return
	}

	resp, err := http.Get(feedURL)
	if err != nil {
		http.Error(w, "failed to fetch podcast feed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		http.Error(w, "failed to fetch podcast feed", http.StatusBadGateway)
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "failed to read feed", http.StatusInternalServerError)
		return
	}

	modified := h.rewriteEnclosures(body, userID, showID)

	h.cache.Set(slug, modified, cache.DefaultExpiration)
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=1800")
	w.Write(modified)
}

// RSS structs
type rssFeed struct {
	XMLName xml.Name `xml:"rss"`
	Channel channel  `xml:"channel"`
}
type channel struct {
	Title       string      `xml:"title"`
	Description string      `xml:"description"`
	Image       rssImage    `xml:"image"`
	ITunesImage itunesImage `xml:"http://www.itunes.com/dtds/podcast-1.0.dtd image"`
	Items       []item      `xml:"item"`
}
type rssImage struct {
	URL string `xml:"url"`
}
type itunesImage struct {
	Href string `xml:"href,attr"`
}

// channelImage prefers the RSS <image><url>, falling back to iTunes artwork.
func channelImage(c channel) string {
	if c.Image.URL != "" {
		return c.Image.URL
	}
	return c.ITunesImage.Href
}

// parseChannelMeta pulls show-level fields from raw feed bytes. Returns empties
// on a parse failure; UpdateShowMeta then leaves existing values untouched.
func parseChannelMeta(feedXML []byte) (title, description, image string) {
	var feed rssFeed
	if err := xml.Unmarshal(feedXML, &feed); err != nil {
		return "", "", ""
	}
	return feed.Channel.Title, feed.Channel.Description, channelImage(feed.Channel)
}
type item struct {
	GUID        string    `xml:"guid"` // CDATA is unwrapped into the string automatically
	Title       string    `xml:"title"`
	Description string    `xml:"description"`
	PubDate     string    `xml:"pubDate"`
	Enclosure   enclosure `xml:"enclosure"`
}
type enclosure struct {
	URL string `xml:"url,attr"` // ",attr" = an XML attribute, not a child element
}

func cleanUpURL(feedUrl string) string {
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

func genSlug(userID string, showID string, showURL string) string {
	// cap to 32 characters
	feedUrlSlug := slug.Make(cleanUpURL(showURL))
	feedUrlSlug = feedUrlSlug[:min(32, len(feedUrlSlug))]

	// take 32 characters of hash
	hashBytes := sha256.Sum256([]byte(userID + showID))
	hashSuffix := fmt.Sprintf("%x", hashBytes)[:8]

	return fmt.Sprintf("%s-%s", feedUrlSlug, hashSuffix)
}
