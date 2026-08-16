package research

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// YouTube endpoints. The player API is YouTube's own internal one — the same
// one the site and yt-dlp use — because there is no public, keyless way to read
// captions. That makes this the most fragile client in the package: it depends
// on YouTube's internals rather than on a contract they publish.
//
// It is contained on purpose. When it breaks it returns an error for YouTube
// URLs and nothing else changes, which is the same shape as any other source
// being unreachable.
const (
	youtubeWatchURL   = "https://www.youtube.com/watch?v="
	youtubePlayerURL  = "https://www.youtube.com/youtubei/v1/player?key="
	youtubeFeedURL    = "https://www.youtube.com/feeds/videos.xml?channel_id="
	youtubeClientName = "ANDROID"
	// youtubeClientVersion has to be recent. A stale version is answered with
	// playabilityStatus UNPLAYABLE and no caption tracks at all — which is a
	// far more confusing failure than a 404, so it is called out here rather
	// than left to be rediscovered.
	youtubeClientVersion = "20.10.38"
)

// youtubeUA is required. YouTube serves a different, captionless page to
// anything that looks like a script.
const youtubeUA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) " +
	"AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

// innertubeKeyPattern pulls the API key out of the watch page.
//
// Scraping it per request rather than hardcoding one is the whole trick: the
// key rotates, and a stale one produces an empty caption list that looks like
// "this video has no subtitles" rather than "you used the wrong key".
var innertubeKeyPattern = regexp.MustCompile(`"INNERTUBE_API_KEY":"([A-Za-z0-9_-]+)"`)

// DefaultYouTubeChannels are the channels the trending sweep watches.
//
// Conference talks are frequently the best source on a subject and often the
// only one — a maintainer explaining why a design went the way it did usually
// says it on stage before anyone writes it down. Channel feeds are plain RSS
// and need no credentials, unlike YouTube search, which has no free keyless
// route at all.
var DefaultYouTubeChannels = []YouTubeChannel{
	{Name: "cncf", ID: "UCvqbFHwN-nwalWPjPUKpvTA"},
	{Name: "goto", ID: "UCT-nPlVzJI-ccQXlxjSvJmw"},
	{Name: "google-cloud-tech", ID: "UCTMRxtyHoE3LPcrl-kT4AQQ"},
}

// YouTubeChannel is one subscribed channel.
type YouTubeChannel struct {
	Name string
	ID   string
}

// YouTubeClient reads video transcripts and channel feeds.
//
// It is a Fetcher and a Lister but deliberately not a Searcher: YouTube search
// has no free keyless route — every Invidious instance tested answered 403, and
// the official Data API needs a Google key. So videos are discovered through
// channel feeds, the same way the engineering blogs are.
type YouTubeClient struct {
	channels []YouTubeChannel
	client   *http.Client
}

// NewYouTubeClient builds a client over the given channels, or
// DefaultYouTubeChannels when the list is empty.
func NewYouTubeClient(channels []YouTubeChannel) *YouTubeClient {
	if len(channels) == 0 {
		channels = DefaultYouTubeChannels
	}
	return &YouTubeClient{
		channels: channels,
		// Generous: the transcript flow is three sequential round trips.
		client: &http.Client{Timeout: 40 * time.Second},
	}
}

// Name identifies this backend.
func (c *YouTubeClient) Name() string { return "youtube" }

// Handles reports whether this fetcher can read rawURL.
func (c *YouTubeClient) Handles(rawURL string) bool {
	return youtubeVideoID(rawURL) != ""
}

// youtubeVideoID extracts the video id from any of YouTube's URL shapes, or ""
// when the URL is not a single video.
func youtubeVideoID(rawURL string) string {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return ""
	}
	host := strings.TrimPrefix(strings.ToLower(u.Host), "www.")

	switch host {
	case "youtu.be":
		return validYouTubeID(strings.Trim(u.Path, "/"))
	case "youtube.com", "m.youtube.com", "music.youtube.com":
		if u.Path == "/watch" {
			return validYouTubeID(u.Query().Get("v"))
		}
		// /embed/ID and /shorts/ID
		for _, prefix := range []string{"/embed/", "/shorts/", "/live/"} {
			if strings.HasPrefix(u.Path, prefix) {
				return validYouTubeID(strings.Trim(strings.TrimPrefix(u.Path, prefix), "/"))
			}
		}
	}
	return ""
}

// youtubeIDPattern is the shape of a video id. Checking it stops a channel or
// playlist URL from being treated as a video.
var youtubeIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)

func validYouTubeID(id string) string {
	if youtubeIDPattern.MatchString(id) {
		return id
	}
	return ""
}

// Fetch returns a video's transcript as a Document.
//
// Three round trips, because YouTube offers no single endpoint for this:
// scrape the current API key from the watch page, ask the player API for the
// caption tracks, then fetch the track itself.
func (c *YouTubeClient) Fetch(ctx context.Context, rawURL string) (*Document, error) {
	videoID := youtubeVideoID(rawURL)
	if videoID == "" {
		return nil, fmt.Errorf("research: youtube: %q is not a video URL", rawURL)
	}

	apiKey, err := c.innertubeKey(ctx, videoID)
	if err != nil {
		return nil, err
	}

	player, err := c.playerData(ctx, videoID, apiKey)
	if err != nil {
		return nil, err
	}
	if status := player.PlayabilityStatus.Status; status != "" && status != "OK" {
		return nil, fmt.Errorf("research: youtube: %s is not playable (%s)", videoID, status)
	}

	track := pickCaptionTrack(player.Captions.Renderer.CaptionTracks)
	if track == "" {
		return nil, fmt.Errorf("research: youtube: %s has no captions", videoID)
	}

	text, err := c.captionText(ctx, track)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("research: youtube: %s returned an empty transcript", videoID)
	}

	title := strings.TrimSpace(player.VideoDetails.Title)
	if title == "" {
		title = "YouTube video " + videoID
	}

	return &Document{
		Source: Source{
			URL:     youtubeWatchURL + videoID,
			Title:   title,
			Site:    "youtube.com",
			Excerpt: truncate(strings.TrimSpace(player.VideoDetails.ShortDescription), 300),
		},
		// The transcript is prefixed with what it is, because a wall of spoken
		// text with no framing reads to a model like an article, and a talk is
		// not an article — attributing "the docs say" to a conference Q&A is
		// exactly the kind of mistake the citation checks cannot catch.
		Text: fmt.Sprintf("# %s\n\n(Transcript of a YouTube video by %s)\n\n%s",
			title, player.VideoDetails.Author, text),
		FetchedAt: time.Now(),
	}, nil
}

// List returns recent videos from the configured channels via their RSS feeds.
func (c *YouTubeClient) List(ctx context.Context, limit int) ([]TrendingItem, error) {
	limit = clampLimit(limit)

	var (
		out      []TrendingItem
		failures []string
	)
	for _, ch := range c.channels {
		items, err := c.channelFeed(ctx, ch)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", ch.Name, err))
			continue
		}
		out = append(out, items...)
	}

	if len(out) == 0 && len(failures) > 0 {
		return nil, fmt.Errorf("research: youtube: every channel feed failed: %s",
			strings.Join(failures, "; "))
	}

	sortByPublished(out)
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (c *YouTubeClient) channelFeed(ctx context.Context, ch YouTubeChannel) ([]TrendingItem, error) {
	body, err := c.get(ctx, youtubeFeedURL+url.QueryEscape(ch.ID))
	if err != nil {
		return nil, err
	}

	// The channel feed is Atom, so the shared decoder handles it. parseFeed
	// itself is not reused because it stamps an "rss:" channel.
	doc, err := decodeFeed(body)
	if err != nil {
		return nil, err
	}

	cutoff := time.Now().Add(-feedMaxAge)
	out := make([]TrendingItem, 0, len(doc.Entries))
	for _, e := range doc.Entries {
		src := e.toSource()
		if youtubeVideoID(src.URL) == "" {
			continue
		}
		if src.PublishedAt != nil && src.PublishedAt.Before(cutoff) {
			continue
		}
		src.Site = "youtube.com"
		out = append(out, TrendingItem{
			Source:  src,
			Score:   0,
			Channel: "youtube:" + ch.Name,
		})
	}
	return out, nil
}

// innertubeKey scrapes the current API key from a watch page.
func (c *YouTubeClient) innertubeKey(ctx context.Context, videoID string) (string, error) {
	body, err := c.get(ctx, youtubeWatchURL+videoID)
	if err != nil {
		return "", err
	}
	m := innertubeKeyPattern.FindSubmatch(body)
	if len(m) < 2 {
		return "", fmt.Errorf(
			"research: youtube: could not find the API key on the watch page for %s — "+
				"YouTube's page layout has probably changed", videoID)
	}
	return string(m[1]), nil
}

// youtubePlayer is the slice of the player response we read.
type youtubePlayer struct {
	PlayabilityStatus struct {
		Status string `json:"status"`
	} `json:"playabilityStatus"`
	VideoDetails struct {
		Title            string `json:"title"`
		Author           string `json:"author"`
		ShortDescription string `json:"shortDescription"`
	} `json:"videoDetails"`
	Captions struct {
		Renderer struct {
			CaptionTracks []struct {
				BaseURL      string `json:"baseUrl"`
				LanguageCode string `json:"languageCode"`
				Kind         string `json:"kind"`
			} `json:"captionTracks"`
		} `json:"playerCaptionsTracklistRenderer"`
	} `json:"captions"`
}

func (c *YouTubeClient) playerData(ctx context.Context, videoID, apiKey string) (*youtubePlayer, error) {
	payload := fmt.Sprintf(
		`{"videoId":%q,"context":{"client":{"clientName":%q,"clientVersion":%q}}}`,
		videoID, youtubeClientName, youtubeClientVersion)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		youtubePlayerURL+url.QueryEscape(apiKey), strings.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("research: youtube: build player request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", youtubeUA)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("research: youtube: player: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("research: youtube: player returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, feedMaxBody))
	if err != nil {
		return nil, fmt.Errorf("research: youtube: read player response: %w", err)
	}

	var player youtubePlayer
	if err := json.Unmarshal(body, &player); err != nil {
		return nil, fmt.Errorf("research: youtube: decode player response: %w", err)
	}
	return &player, nil
}

// pickCaptionTrack prefers a human-written English track, then any English one,
// then whatever exists.
//
// Auto-generated captions are usable but noticeably worse — no punctuation,
// mangled proper nouns — and a quote pulled from one is a quote that will not
// survive the verification pass cleanly.
func pickCaptionTrack(tracks []struct {
	BaseURL      string `json:"baseUrl"`
	LanguageCode string `json:"languageCode"`
	Kind         string `json:"kind"`
}) string {
	var anyEnglish, anyTrack string
	for _, t := range tracks {
		if t.BaseURL == "" {
			continue
		}
		if anyTrack == "" {
			anyTrack = t.BaseURL
		}
		if !strings.HasPrefix(t.LanguageCode, "en") {
			continue
		}
		if anyEnglish == "" {
			anyEnglish = t.BaseURL
		}
		// kind "asr" marks an automatic transcription.
		if t.Kind != "asr" {
			return t.BaseURL
		}
	}
	if anyEnglish != "" {
		return anyEnglish
	}
	return anyTrack
}

// timedText is YouTube's caption format: timed <p> elements.
type timedText struct {
	Paragraphs []struct {
		Text string `xml:",chardata"`
	} `xml:"body>p"`
}

func (c *YouTubeClient) captionText(ctx context.Context, trackURL string) (string, error) {
	body, err := c.get(ctx, trackURL)
	if err != nil {
		return "", err
	}

	var doc timedText
	if err := xml.Unmarshal(body, &doc); err != nil {
		return "", fmt.Errorf("research: youtube: decode captions: %w", err)
	}

	var b strings.Builder
	for _, p := range doc.Paragraphs {
		// Caption text is HTML-escaped, and runs together without spacing
		// because each <p> is a timing cue rather than a sentence.
		if s := strings.TrimSpace(html.UnescapeString(p.Text)); s != "" {
			b.WriteString(s)
			b.WriteByte(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " "), nil
}

func (c *YouTubeClient) get(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("research: youtube: build request: %w", err)
	}
	req.Header.Set("User-Agent", youtubeUA)
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("research: youtube: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("research: youtube: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, feedMaxBody))
	if err != nil {
		return nil, fmt.Errorf("research: youtube: read body: %w", err)
	}
	return body, nil
}
