package model

import (
	"time"

	"github.com/google/uuid"
)

const (
	BuiltinSEOAnalyst = "seo_analyst"
	AgentNameSEO      = "SEO Analyst"
)

// CreateSEORunRequest is the launch payload for an SEO Analyst run.
type CreateSEORunRequest struct {
	AgentID     uuid.UUID  `json:"agent_id" validate:"required"`
	TaskID      *uuid.UUID `json:"task_id"`
	URL         string     `json:"url" validate:"required"`
	Mode        string     `json:"mode" validate:"required,oneof=analyze improve publish"`
	Platform    string     `json:"platform" validate:"omitempty,oneof=auto wordpress opsapi static"`
	GitRepo     string     `json:"git_repo"`     // owner/repo or https URL
	GitBranch   string     `json:"git_branch"`   // base branch, default main
	SiteMapPath string     `json:"sitemap_path"` // default sitemap.xml
	Keywords    string     `json:"keywords"`     // comma-separated focus keywords
	Instruction string     `json:"instruction" validate:"omitempty,max=4000"`
}

// SEORun is one analysis / improve / publish execution.
type SEORun struct {
	ID           uuid.UUID      `json:"id"`
	AgentID      uuid.UUID      `json:"agent_id"`
	TaskID       *uuid.UUID     `json:"task_id"`
	OrgID        uuid.UUID      `json:"org_id"`
	Status       string         `json:"status"` // queued, running, completed, failed, cancelled
	Mode         string         `json:"mode"`
	URL          string         `json:"url"`
	Platform     string         `json:"platform"`
	DetectedCMS  string         `json:"detected_cms,omitempty"`
	GitRepo      string         `json:"git_repo,omitempty"`
	GitBranch    string         `json:"git_branch,omitempty"`
	Keywords     string         `json:"keywords,omitempty"`
	Instruction  *string        `json:"instruction,omitempty"`
	Score        *SEOScore      `json:"score,omitempty"`
	Report       *SEOReport     `json:"report,omitempty"`
	Publish      *SEOPublishResult `json:"publish,omitempty"`
	ErrorMessage *string        `json:"error_message,omitempty"`
	RequestedBy  *uuid.UUID     `json:"requested_by"`
	StartedAt    *time.Time     `json:"started_at"`
	CompletedAt  *time.Time     `json:"completed_at"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

// SEOScore summarises crawl health.
type SEOScore struct {
	Overall        int            `json:"overall"` // 0–100
	TitleOK        bool           `json:"title_ok"`
	MetaDescOK     bool           `json:"meta_desc_ok"`
	H1OK           bool           `json:"h1_ok"`
	CanonicalOK    bool           `json:"canonical_ok"`
	RobotsOK       bool           `json:"robots_ok"`
	SitemapOK      bool           `json:"sitemap_ok"`
	OpenGraphOK    bool           `json:"open_graph_ok"`
	HTTPSOK        bool           `json:"https_ok"`
	ByCategory     map[string]int `json:"by_category,omitempty"`
	IssueCount     int            `json:"issue_count"`
	CriticalCount  int            `json:"critical_count"`
}

// SEOReport is the structured analysis payload stored on the run.
type SEOReport struct {
	FinalURL       string            `json:"final_url"`
	StatusCode     int               `json:"status_code"`
	Title          string            `json:"title"`
	MetaDescription string           `json:"meta_description"`
	Canonical      string            `json:"canonical"`
	H1             []string          `json:"h1"`
	H2             []string          `json:"h2,omitempty"`
	KeywordsFound  []string          `json:"keywords_found,omitempty"`
	RobotsTxt      string            `json:"robots_txt,omitempty"`
	SitemapURL     string            `json:"sitemap_url,omitempty"`
	SitemapFound   bool              `json:"sitemap_found"`
	OpenGraph      map[string]string `json:"open_graph,omitempty"`
	Issues         []SEOIssue        `json:"issues"`
	Suggestions    []SEOSuggestion   `json:"suggestions"`
	ProposedSlug   string            `json:"proposed_slug,omitempty"`
	SitemapXML     string            `json:"sitemap_xml,omitempty"`
	MetaPatch      map[string]string `json:"meta_patch,omitempty"`
	SiteStructure  *SEOSiteStructure `json:"site_structure,omitempty"`
}

// SEOIssue is one finding from analysis.
type SEOIssue struct {
	ID       string `json:"id"`
	Severity string `json:"severity"` // critical, high, medium, low, info
	Category string `json:"category"`
	Title    string `json:"title"`
	Detail   string `json:"detail"`
	FixHint  string `json:"fix_hint,omitempty"`
}

// SEOSuggestion is an actionable improve step.
type SEOSuggestion struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"` // meta, slug, sitemap, content, robots, og
	Title    string `json:"title"`
	Detail   string `json:"detail"`
	Proposed string `json:"proposed,omitempty"`
}

// SEOSiteStructure is detected layout for static/git sites (from README/md skills).
type SEOSiteStructure struct {
	ContentRoot   string   `json:"content_root,omitempty"`
	PublicRoot    string   `json:"public_root,omitempty"`
	TemplateHint  string   `json:"template_hint,omitempty"`
	Pages         []string `json:"pages,omitempty"`
	HasSitemap    bool     `json:"has_sitemap"`
	SkillNotes    []string `json:"skill_notes,omitempty"`
}

// SEOPublishResult records how improvements were applied.
type SEOPublishResult struct {
	Channel    string `json:"channel"` // wordpress | opsapi | git_pr | none
	Success    bool   `json:"success"`
	Message    string `json:"message"`
	PRURL      string `json:"pr_url,omitempty"`
	PRNumber   int    `json:"pr_number,omitempty"`
	RemoteRef  string `json:"remote_ref,omitempty"`
	Fallback   string `json:"fallback,omitempty"` // e.g. api_failed_used_git_pr
}
