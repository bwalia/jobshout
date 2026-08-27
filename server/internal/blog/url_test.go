package blog

import "testing"

func TestPublicImageURL(t *testing.T) {
	tests := []struct {
		name, base, url, want string
	}{
		{
			name: "relative cover becomes absolute",
			base: "https://int.jobshout.co.uk",
			url:  "/api/v1/images/file/org/2026/08/abc.png",
			want: "https://int.jobshout.co.uk/api/v1/images/file/org/2026/08/abc.png",
		},
		{
			name: "trailing slash on base is trimmed",
			base: "https://int.jobshout.co.uk/",
			url:  "/api/v1/images/file/x.png",
			want: "https://int.jobshout.co.uk/api/v1/images/file/x.png",
		},
		{
			name: "already absolute is left alone",
			base: "https://int.jobshout.co.uk",
			url:  "https://cdn.example.com/cover.png",
			want: "https://cdn.example.com/cover.png",
		},
		{
			name: "empty cover stays empty",
			base: "https://int.jobshout.co.uk",
			url:  "",
			want: "",
		},
		{
			name: "no public base omits the cover rather than sending a relative path",
			base: "",
			url:  "/api/v1/images/file/x.png",
			want: "",
		},
		{
			name: "missing leading slash is added",
			base: "https://int.jobshout.co.uk",
			url:  "api/v1/images/file/x.png",
			want: "https://int.jobshout.co.uk/api/v1/images/file/x.png",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := publicImageURL(tt.base, tt.url); got != tt.want {
				t.Errorf("publicImageURL(%q, %q) = %q, want %q", tt.base, tt.url, got, tt.want)
			}
		})
	}
}
