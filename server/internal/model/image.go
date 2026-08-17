package model

import (
	"time"

	"github.com/google/uuid"
)

// Image sources — what asked for a generated image. Recorded so the log of what
// the platform has drawn can be read by cause, not just by time.
const (
	// ImageSourceBlogCover is an article's cover image.
	ImageSourceBlogCover = "blog_cover"
	// ImageSourceBlogInline is an illustration inside an article's body.
	ImageSourceBlogInline = "blog_inline"
	// ImageSourceAgentTool is an agent calling the generate_image tool.
	ImageSourceAgentTool = "agent_tool"
	// ImageSourceManual is a person asking for one in the UI.
	ImageSourceManual = "manual"
)

// GeneratedImage records one image the platform produced.
type GeneratedImage struct {
	ID    uuid.UUID `json:"id"`
	OrgID uuid.UUID `json:"org_id"`
	// CreatedBy is nil for images produced by a schedule or by an agent acting
	// on its own — there is no person behind those, and naming one would be
	// wrong rather than merely imprecise.
	CreatedBy *uuid.UUID `json:"created_by,omitempty"`

	Prompt   string `json:"prompt"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
	// Seed is what makes an image reproducible. -1 means the provider chose one
	// and did not report it, which is the case for hosted APIs.
	Seed   int64 `json:"seed"`
	Width  int   `json:"width"`
	Height int   `json:"height"`

	// URL is where the image is served from, or empty when object storage was
	// not configured and the bytes were handed straight back to the caller.
	URL        string    `json:"url,omitempty"`
	Source     string    `json:"source"`
	DurationMS int       `json:"duration_ms"`
	CreatedAt  time.Time `json:"created_at"`
}

// CoverImage is an article's cover: where it is, what asked for it, and enough
// detail to draw it again.
type CoverImage struct {
	URL    string `json:"url"`
	Prompt string `json:"prompt"`
	Meta   CoverImageMeta `json:"meta"`
}

// CoverImageMeta is the settings behind a cover image, stored as JSON on the
// article. Kept whole rather than split into columns because it is written and
// read whole, and a provider that grows a new setting should not need a
// migration.
type CoverImageMeta struct {
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
	Seed     int64  `json:"seed,omitempty"`
	Width    int    `json:"width,omitempty"`
	Height   int    `json:"height,omitempty"`
}
