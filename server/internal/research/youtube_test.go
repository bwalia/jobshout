package research

import "testing"

func TestYouTubeVideoID(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"watch", "https://www.youtube.com/watch?v=aircAruvnKk", "aircAruvnKk"},
		{"watch with extras", "https://www.youtube.com/watch?v=aircAruvnKk&t=42s&list=PL1", "aircAruvnKk"},
		{"short link", "https://youtu.be/aircAruvnKk", "aircAruvnKk"},
		{"short link with query", "https://youtu.be/aircAruvnKk?t=42", "aircAruvnKk"},
		{"embed", "https://www.youtube.com/embed/aircAruvnKk", "aircAruvnKk"},
		{"shorts", "https://www.youtube.com/shorts/aircAruvnKk", "aircAruvnKk"},
		{"live", "https://www.youtube.com/live/aircAruvnKk", "aircAruvnKk"},
		{"mobile", "https://m.youtube.com/watch?v=aircAruvnKk", "aircAruvnKk"},
		{"no www", "https://youtube.com/watch?v=aircAruvnKk", "aircAruvnKk"},

		// Not single videos. Claiming these would mean fetching a transcript
		// for something that has none, and reporting that as a dead source.
		{"channel", "https://www.youtube.com/@cncf", ""},
		{"playlist", "https://www.youtube.com/playlist?list=PL1234", ""},
		{"results page", "https://www.youtube.com/results?search_query=kubernetes", ""},
		{"not youtube", "https://kubernetes.io/blog/", ""},
		{"malformed id", "https://www.youtube.com/watch?v=tooshort", ""},
		{"unparseable", "://nope", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := youtubeVideoID(tt.in); got != tt.want {
				t.Errorf("youtubeVideoID(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestYouTubeClient_HandlesOnlyVideos(t *testing.T) {
	c := NewYouTubeClient(nil)
	if !c.Handles("https://www.youtube.com/watch?v=aircAruvnKk") {
		t.Error("did not claim a video URL")
	}
	if c.Handles("https://www.youtube.com/@cncf") {
		t.Error("claimed a channel URL it cannot fetch a transcript for")
	}
}

// captionTrack mirrors the anonymous struct pickCaptionTrack takes.
type captionTrack = struct {
	BaseURL      string `json:"baseUrl"`
	LanguageCode string `json:"languageCode"`
	Kind         string `json:"kind"`
}

func TestPickCaptionTrack(t *testing.T) {
	tests := []struct {
		name   string
		tracks []captionTrack
		want   string
	}{
		{
			// Human-written English beats an auto-generated one: ASR output has
			// no punctuation and mangles proper nouns, and a quote pulled from
			// it will not survive verification cleanly.
			name: "prefers human English over auto-generated",
			tracks: []captionTrack{
				{BaseURL: "auto", LanguageCode: "en", Kind: "asr"},
				{BaseURL: "manual", LanguageCode: "en"},
			},
			want: "manual",
		},
		{
			name: "falls back to auto-generated English",
			tracks: []captionTrack{
				{BaseURL: "auto", LanguageCode: "en", Kind: "asr"},
				{BaseURL: "german", LanguageCode: "de"},
			},
			want: "auto",
		},
		{
			name:   "falls back to any track",
			tracks: []captionTrack{{BaseURL: "german", LanguageCode: "de"}},
			want:   "german",
		},
		{
			name:   "no tracks",
			tracks: nil,
			want:   "",
		},
		{
			name:   "ignores tracks with no url",
			tracks: []captionTrack{{LanguageCode: "en"}},
			want:   "",
		},
		{
			name: "en-GB counts as English",
			tracks: []captionTrack{
				{BaseURL: "german", LanguageCode: "de"},
				{BaseURL: "british", LanguageCode: "en-GB"},
			},
			want: "british",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pickCaptionTrack(tt.tracks); got != tt.want {
				t.Errorf("pickCaptionTrack() = %q, want %q", got, tt.want)
			}
		})
	}
}

// A transcript must be labelled as one. A wall of spoken text with no framing
// reads to a model like an article, and attributing "the documentation says" to
// a conference Q&A is exactly what the citation checks cannot catch.
func TestYouTubeChannelsAreConfigured(t *testing.T) {
	c := NewYouTubeClient(nil)
	if len(c.channels) == 0 {
		t.Fatal("no default channels configured")
	}
	for _, ch := range c.channels {
		if ch.ID == "" || ch.Name == "" {
			t.Errorf("channel %+v is incomplete", ch)
		}
	}
}

func TestNewYouTubeClient_UsesGivenChannels(t *testing.T) {
	c := NewYouTubeClient([]YouTubeChannel{{Name: "custom", ID: "UC123"}})
	if len(c.channels) != 1 || c.channels[0].Name != "custom" {
		t.Errorf("got channels %+v, want the supplied one", c.channels)
	}
}
