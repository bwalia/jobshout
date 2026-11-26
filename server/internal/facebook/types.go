package facebook

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// Builtin and display name of the specialist.
const (
	Builtin   = "facebook_page_manager"
	AgentName = "Facebook Page Manager"
)

// Post statuses.
//
//	drafting → draft → scheduled → publishing → published
//	      ↘ failed ↙        ↘ draft (Facebook rejected it; the error says why)
//
// publishing is held while the Graph call is in flight, so a double click or a
// second replica cannot publish the same post twice.
const (
	StatusDrafting   = "drafting"
	StatusDraft      = "draft"
	StatusScheduled  = "scheduled"
	StatusPublishing = "publishing"
	StatusPublished  = "published"
	StatusFailed     = "failed"
)

// MessageMax is the length the editor allows. Facebook accepts far more, but
// a post that long is not a good post.
const MessageMax = 5000

var (
	// ErrNotFound: a post, Page or connection that is not in the caller's org.
	ErrNotFound = errors.New("facebook: not found")
	// ErrConflict: the post is not in a state that allows the change.
	ErrConflict = errors.New("facebook: post is not in a state that allows this")
	// ErrNotConfigured: the deployment has no Meta app.
	ErrNotConfigured = errors.New("Facebook is not configured on this deployment (FACEBOOK_APP_ID, FACEBOOK_APP_SECRET, FACEBOOK_TOKEN_KEY)")
	// ErrNotConnected: nobody in the org has connected a Page.
	ErrNotConnected = errors.New("connect Facebook on the Facebook Page Manager tab first")
)

// Post is one calendar post for one Page.
type Post struct {
	ID          uuid.UUID  `json:"id"`
	OrgID       uuid.UUID  `json:"org_id"`
	PageID      string     `json:"page_id"`
	TaskID      *uuid.UUID `json:"task_id,omitempty"`
	Status      string     `json:"status"`
	Message     string     `json:"message"`
	ImageURL    string     `json:"image_url,omitempty"`
	ImagePrompt string     `json:"image_prompt,omitempty"`
	Category    string     `json:"category,omitempty"`
	Audience    string     `json:"audience,omitempty"`
	Objective   string     `json:"objective,omitempty"`
	Notes       string     `json:"notes,omitempty"`
	// ScheduledFor is the suggested time on a draft and the publish time on a
	// scheduled post.
	ScheduledFor *time.Time `json:"scheduled_for,omitempty"`
	ErrorMessage string     `json:"error_message,omitempty"`
	FBPostID     string     `json:"fb_post_id,omitempty"`
	PostURL      string     `json:"post_url,omitempty"`
	PublishedAt  *time.Time `json:"published_at,omitempty"`
	ApprovedBy   *uuid.UUID `json:"approved_by,omitempty"`
	Reactions    int        `json:"reactions"`
	Comments     int        `json:"comments"`
	Shares       int        `json:"shares"`
	StatsAt      *time.Time `json:"stats_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// Engagement is reactions, comments and shares together.
func (p Post) Engagement() int { return p.Reactions + p.Comments + p.Shares }

// PageRecord is a connected Page. The access token is never part of it.
type PageRecord struct {
	ID          uuid.UUID `json:"id"`
	OrgID       uuid.UUID `json:"org_id"`
	PageID      string    `json:"page_id"`
	Name        string    `json:"name"`
	Category    string    `json:"category"`
	About       string    `json:"about"`
	Description string    `json:"description"`
	Website     string    `json:"website"`
	Followers   int       `json:"followers"`
	NeedsReauth bool      `json:"needs_reauth"`
	// Brief, Voice and Timezone are this Page's own profile: what the business
	// is, how it sounds, and the clock its posting times are read on.
	Brief    string `json:"brief"`
	Voice    string `json:"voice"`
	Timezone string `json:"timezone"`
	// Paused stops scheduled posts publishing for this Page until resumed.
	Paused    bool      `json:"paused"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Connection is the Facebook user an org connected.
type Connection struct {
	OrgID       uuid.UUID  `json:"org_id"`
	FBUserID    string     `json:"fb_user_id"`
	Name        string     `json:"name"`
	ConnectedBy *uuid.UUID `json:"connected_by,omitempty"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
}

// Status is what the agent's tab shows about the connection.
type Status struct {
	// Configured is false when the deployment has no Meta app.
	Configured bool         `json:"configured"`
	Connected  bool         `json:"connected"`
	Name       string       `json:"name,omitempty"`
	ExpiresAt  *time.Time   `json:"expires_at,omitempty"`
	Pages      []PageRecord `json:"pages"`
	// PageCreation is always unsupported: Meta's Graph API has no call that
	// creates a Page. The tab shows the manual steps instead.
	PageCreation PageCreation `json:"page_creation"`
}

// PageCreation explains why the agent cannot create a Page and what to do.
type PageCreation struct {
	Supported bool     `json:"supported"`
	Reason    string   `json:"reason"`
	Steps     []string `json:"steps"`
}

// CreationGuide is the manual Page setup workflow.
var CreationGuide = PageCreation{
	Supported: false,
	Reason:    "Meta's Graph API cannot create Pages, so this agent never claims to. Create the Page yourself, then connect it here.",
	Steps: []string{
		"In Facebook, open Pages → Create new Page and enter the business name, category and bio.",
		"Add the profile picture and cover image (Meta does not let apps change these).",
		"Choose Connect Facebook on this tab and accept every permission for that Page.",
		"Open the Page's profile here and add a short business brief and brand voice so posts match the brand.",
	},
}

// Analytics is the performance summary over recent published posts.
type Analytics struct {
	PageID     string           `json:"page_id"`
	Posts      int              `json:"posts"`
	Reactions  int              `json:"reactions"`
	Comments   int              `json:"comments"`
	Shares     int              `json:"shares"`
	AvgPerPost float64          `json:"avg_engagement_per_post"`
	Top        *Post            `json:"top_post,omitempty"`
	ByCategory []CategoryResult `json:"by_category"`
	Followers  int              `json:"followers"`
}

// CategoryResult is how one content category performed.
type CategoryResult struct {
	Category   string  `json:"category"`
	Posts      int     `json:"posts"`
	AvgPerPost float64 `json:"avg_engagement_per_post"`
}
