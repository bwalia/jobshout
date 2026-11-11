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
	AgentID          uuid.UUID  `json:"agent_id" validate:"required"`
	TaskID           *uuid.UUID `json:"task_id"`
	Source           string     `json:"source" validate:"omitempty,oneof=urls github"` // urls | github
	URL              string     `json:"url"`                                          // single URL (legacy) or first of list
	URLs             string     `json:"urls"`                                         // textarea: one URL per line
	Mode             string     `json:"mode" validate:"required,oneof=analyze improve publish"`
	Platform         string     `json:"platform" validate:"omitempty,oneof=auto wordpress opsapi static"`
	GitRepo          string     `json:"git_repo"`          // owner/repo (optional override of connection.repo)
	GitBranch        string     `json:"git_branch"`        // base branch, default main
	GitHubConnection string     `json:"github_connection"` // named server-side credential
	PathGlob         string     `json:"path_glob"`         // e.g. content/**/*.md
	MaxPages         int        `json:"max_pages"`         // cap pages analysed
	SiteMapPath      string     `json:"sitemap_path"`      // default sitemap.xml
	Keywords         string     `json:"keywords"`          // focus keywords (comma / newline)
	Instruction      string     `json:"instruction" validate:"omitempty,max=4000"`
}

// SEORun is one analysis / improve / publish execution.
type SEORun struct {
	ID               uuid.UUID         `json:"id"`
	AgentID          uuid.UUID         `json:"agent_id"`
	TaskID           *uuid.UUID        `json:"task_id"`
	OrgID            uuid.UUID         `json:"org_id"`
	Status           string            `json:"status"` // queued, running, completed, failed, cancelled
	Mode             string            `json:"mode"`   // analyze | improve | publish
	Source           string            `json:"source"` // urls | github
	URL              string            `json:"url"`
	Platform         string            `json:"platform"`
	DetectedCMS      string            `json:"detected_cms,omitempty"`
	GitRepo          string            `json:"git_repo,omitempty"`
	GitBranch        string            `json:"git_branch,omitempty"`
	GitHubConnection string            `json:"github_connection,omitempty"`
	PathGlob         string            `json:"path_glob,omitempty"`
	MaxPages         int               `json:"max_pages,omitempty"`
	Keywords         string            `json:"keywords,omitempty"`
	Instruction      *string           `json:"instruction,omitempty"`
	Score            *SEOScore         `json:"score,omitempty"`
	Report           *SEOReport        `json:"report,omitempty"`
	Pages            []SEOPageReport   `json:"pages,omitempty"`
	Progress         *SEOProgress      `json:"progress,omitempty"`
	Publish          *SEOPublishResult `json:"publish,omitempty"`
	ErrorMessage     *string           `json:"error_message,omitempty"`
	RequestedBy      *uuid.UUID        `json:"requested_by"`
	StartedAt        *time.Time        `json:"started_at"`
	CompletedAt      *time.Time        `json:"completed_at"`
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
}

// SEOProgress tracks multi-page run state for the Workspace UI.
type SEOProgress struct {
	Total     int    `json:"total"`
	Completed int    `json:"completed"`
	Current   string `json:"current,omitempty"`
	Phase     string `json:"phase,omitempty"` // listing | analysing | publishing | done
}

// SEOPageReport is one page in a multi-page run.
type SEOPageReport struct {
	Path            string            `json:"path"`
	SourceURL       string            `json:"source_url,omitempty"`
	Status          string            `json:"status"` // queued | running | completed | failed
	Error           string            `json:"error,omitempty"`
	Title           string            `json:"title,omitempty"`
	MetaDescription string            `json:"meta_description,omitempty"`
	H1              []string          `json:"h1,omitempty"`
	KeywordsFound   []string          `json:"keywords_found,omitempty"`
	Score           *SEOScore         `json:"score,omitempty"`
	Issues          []SEOIssue        `json:"issues,omitempty"`
	Suggestions     []SEOSuggestion   `json:"suggestions,omitempty"`
	MetaPatch       map[string]string `json:"meta_patch,omitempty"`
	ProposedDiff    string            `json:"proposed_diff,omitempty"`
}

// SEOScore summarises crawl health.
type SEOScore struct {
	Overall       int            `json:"overall"` // 0–100
	TitleOK       bool           `json:"title_ok"`
	MetaDescOK    bool           `json:"meta_desc_ok"`
	H1OK          bool           `json:"h1_ok"`
	CanonicalOK   bool           `json:"canonical_ok"`
	RobotsOK      bool           `json:"robots_ok"`
	SitemapOK     bool           `json:"sitemap_ok"`
	OpenGraphOK   bool           `json:"open_graph_ok"`
	HTTPSOK       bool           `json:"https_ok"`
	ByCategory    map[string]int `json:"by_category,omitempty"`
	IssueCount    int            `json:"issue_count"`
	CriticalCount int            `json:"critical_count"`
}

// SEOReport is the structured analysis payload stored on the run (rollup + first page).
type SEOReport struct {
	FinalURL        string            `json:"final_url"`
	StatusCode      int               `json:"status_code"`
	Title           string            `json:"title"`
	MetaDescription string            `json:"meta_description"`
	Canonical       string            `json:"canonical"`
	H1              []string          `json:"h1"`
	H2              []string          `json:"h2,omitempty"`
	KeywordsFound   []string          `json:"keywords_found,omitempty"`
	RobotsTxt       string            `json:"robots_txt,omitempty"`
	SitemapURL      string            `json:"sitemap_url,omitempty"`
	SitemapFound    bool              `json:"sitemap_found"`
	OpenGraph       map[string]string `json:"open_graph,omitempty"`
	Issues          []SEOIssue        `json:"issues"`
	Suggestions     []SEOSuggestion   `json:"suggestions"`
	ProposedSlug    string            `json:"proposed_slug,omitempty"`
	SitemapXML      string            `json:"sitemap_xml,omitempty"`
	MetaPatch       map[string]string `json:"meta_patch,omitempty"`
	SiteStructure   *SEOSiteStructure `json:"site_structure,omitempty"`
	PageCount       int               `json:"page_count,omitempty"`
	Keywords        []string          `json:"keywords,omitempty"`
	RollupSummary   string            `json:"rollup_summary,omitempty"`
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
	ContentRoot  string   `json:"content_root,omitempty"`
	PublicRoot   string   `json:"public_root,omitempty"`
	TemplateHint string   `json:"template_hint,omitempty"`
	Pages        []string `json:"pages,omitempty"`
	HasSitemap   bool     `json:"has_sitemap"`
	SkillNotes   []string `json:"skill_notes,omitempty"`
}

// SEOPublishResult records how improvements were applied.
type SEOPublishResult struct {
	Channel   string `json:"channel"` // wordpress | opsapi | git_pr | none | suggested_diff
	Success   bool   `json:"success"`
	Message   string `json:"message"`
	PRURL     string `json:"pr_url,omitempty"`
	PRNumber  int    `json:"pr_number,omitempty"`
	RemoteRef string `json:"remote_ref,omitempty"`
	Fallback  string `json:"fallback,omitempty"` // e.g. api_failed_used_git_pr
}
