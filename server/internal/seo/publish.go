package seo

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	githubAdapter "github.com/jobshout/server/internal/integration/adapters/github"
	"github.com/jobshout/server/internal/integration/adapters/opsapi"
)

// PublishInput drives CMS / git publish after improve.
type PublishInput struct {
	SiteURL      string
	Platform     string // wordpress | opsapi | static | auto
	DetectedCMS  string
	GitRepo      string // owner/repo
	GitBranch    string // base
	SitemapPath  string
	SitemapXML   string
	MetaPatch    map[string]string
	ProposedSlug string
	Keywords     []string
	Structure    *SiteStructure
	OpsAPIBase   string
	OpsAPIKey    string
	OpsNamespace string
	WPUser       string
	WPAppPassword string
}

// PublishResult is returned to the service layer.
type PublishResult struct {
	Channel   string
	Success   bool
	Message   string
	PRURL     string
	PRNumber  int
	RemoteRef string
	Fallback  string
}

// Publish applies SEO improvements via the best available channel.
// Order: explicit platform API → detected CMS API → git PR fallback when GitRepo is set.
func Publish(ctx context.Context, in PublishInput) PublishResult {
	platform := strings.ToLower(strings.TrimSpace(in.Platform))
	if platform == "" || platform == "auto" {
		platform = strings.ToLower(strings.TrimSpace(in.DetectedCMS))
	}
	if in.SitemapPath == "" {
		in.SitemapPath = "sitemap.xml"
	}
	if in.GitBranch == "" {
		in.GitBranch = "main"
	}

	switch platform {
	case "wordpress":
		if r := publishWordPress(ctx, in); r.Success {
			return r
		} else if in.GitRepo != "" {
			r2 := publishGitPR(ctx, in)
			r2.Fallback = "api_failed_used_git_pr"
			if r2.Message == "" {
				r2.Message = r.Message + "; fell back to git PR"
			} else {
				r2.Message = r.Message + "; " + r2.Message
			}
			return r2
		} else {
			return r
		}
	case "opsapi":
		if r := publishOpsAPI(ctx, in); r.Success {
			return r
		} else if in.GitRepo != "" {
			r2 := publishGitPR(ctx, in)
			r2.Fallback = "api_failed_used_git_pr"
			r2.Message = r.Message + "; " + r2.Message
			return r2
		} else {
			return r
		}
	case "static", "unknown", "":
		if in.GitRepo != "" {
			return publishGitPR(ctx, in)
		}
		return PublishResult{
			Channel: "none",
			Success: false,
			Message: "No git_repo provided and no CMS credentials — analysis/improve only. Pass git_repo (owner/repo) to open a PR with sitemap + meta patches.",
		}
	default:
		if in.GitRepo != "" {
			return publishGitPR(ctx, in)
		}
		return PublishResult{Channel: "none", Success: false, Message: "Unsupported platform " + platform}
	}
}

func publishWordPress(ctx context.Context, in PublishInput) PublishResult {
	user := firstEnv(in.WPUser, "SEO_WORDPRESS_USER", "WORDPRESS_USER")
	pass := firstEnv(in.WPAppPassword, "SEO_WORDPRESS_APP_PASSWORD", "WORDPRESS_APP_PASSWORD")
	if user == "" || pass == "" {
		return PublishResult{Channel: "wordpress", Success: false, Message: "WordPress credentials missing (SEO_WORDPRESS_USER / SEO_WORDPRESS_APP_PASSWORD)"}
	}
	base := strings.TrimRight(in.SiteURL, "/")
	// Best-effort: create/update a draft page titled from meta patch via WP REST.
	title := in.MetaPatch["title"]
	if title == "" {
		title = "SEO update " + time.Now().UTC().Format("2006-01-02")
	}
	content := "<p>SEO Analyst proposed updates.</p>"
	if d := in.MetaPatch["meta_description"]; d != "" {
		content += "<p>" + d + "</p>"
	}
	if in.SitemapXML != "" {
		content += "\n<!-- sitemap.xml draft attached in git PR if configured -->\n"
	}
	payload := map[string]any{
		"title":   title,
		"status":  "draft",
		"slug":    in.ProposedSlug,
		"content": content,
		"excerpt": in.MetaPatch["meta_description"],
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/wp-json/wp/v2/pages", bytes.NewReader(body))
	if err != nil {
		return PublishResult{Channel: "wordpress", Success: false, Message: err.Error()}
	}
	req.SetBasicAuth(user, pass)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return PublishResult{Channel: "wordpress", Success: false, Message: err.Error()}
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode >= 300 {
		return PublishResult{Channel: "wordpress", Success: false, Message: fmt.Sprintf("WP REST %d: %s", resp.StatusCode, truncate(string(b), 200))}
	}
	return PublishResult{Channel: "wordpress", Success: true, Message: "Created WordPress draft page with proposed SEO title/slug/excerpt"}
}

func publishOpsAPI(ctx context.Context, in PublishInput) PublishResult {
	base := firstEnv(in.OpsAPIBase, "OPSAPI_BASE_URL", "CMS_BASE_URL")
	key := firstEnv(in.OpsAPIKey, "OPSAPI_API_KEY", "CMS_API_KEY")
	ns := firstEnv(in.OpsNamespace, "OPSAPI_NAMESPACE", "CMS_NAMESPACE")
	if ns == "" {
		ns = "default"
	}
	if base == "" || key == "" {
		return PublishResult{Channel: "opsapi", Success: false, Message: "OpsAPI not configured (OPSAPI_BASE_URL / OPSAPI_API_KEY)"}
	}
	client := opsapi.NewClient(opsapi.Config{BaseURL: base, APIKey: key, Namespace: ns})
	if client == nil {
		return PublishResult{Channel: "opsapi", Success: false, Message: "OpsAPI client incomplete (need base URL, API key, and namespace)"}
	}
	title := in.MetaPatch["title"]
	if title == "" {
		title = "SEO update"
	}
	body := in.MetaPatch["meta_description"]
	if body == "" {
		body = "SEO Analyst draft"
	}
	post, err := client.CreatePost(ctx, opsapi.CreatePostRequest{
		Title:          title,
		ContentHTML:    "<p>" + body + "</p>",
		Status:         "draft",
		Slug:           in.ProposedSlug,
		Excerpt:        in.MetaPatch["meta_description"],
		SEOTitle:       in.MetaPatch["title"],
		SEODescription: in.MetaPatch["meta_description"],
		Tags:           in.Keywords,
	})
	if err != nil {
		return PublishResult{Channel: "opsapi", Success: false, Message: err.Error()}
	}
	msg := "Created OpsAPI draft with seo_title / seo_description"
	if post != nil && post.UUID != "" {
		msg += " (id " + post.UUID + ")"
	}
	return PublishResult{Channel: "opsapi", Success: true, Message: msg}
}

func publishGitPR(ctx context.Context, in PublishInput) PublishResult {
	token := strings.TrimSpace(os.Getenv("GITHUB_TOKEN"))
	if token == "" {
		token = strings.TrimSpace(os.Getenv("GH_TOKEN"))
	}
	if token == "" {
		return PublishResult{Channel: "git_pr", Success: false, Message: "GITHUB_TOKEN not set — cannot open PR"}
	}
	owner, repo, err := parseRepo(in.GitRepo)
	if err != nil {
		return PublishResult{Channel: "git_pr", Success: false, Message: err.Error()}
	}
	base := in.GitBranch
	if base == "" {
		base = "main"
	}
	branch := fmt.Sprintf("seo/improve-%s", time.Now().UTC().Format("20060102-150405"))
	gh := &gitHubFiles{Token: token, HTTP: &http.Client{Timeout: 45 * time.Second}}

	if err := gh.ensureBranch(ctx, owner, repo, base, branch); err != nil {
		return PublishResult{Channel: "git_pr", Success: false, Message: "create branch: " + err.Error()}
	}

	files := map[string]string{}
	sitemapPath := in.SitemapPath
	if in.Structure != nil && in.Structure.PublicRoot != "" && !strings.Contains(sitemapPath, "/") {
		sitemapPath = strings.TrimSuffix(in.Structure.PublicRoot, "/") + "/" + sitemapPath
	}
	if in.SitemapXML != "" {
		files[sitemapPath] = in.SitemapXML
	}
	// robots.txt Sitemap: line helper
	robots := "User-agent: *\nAllow: /\nSitemap: " + strings.TrimRight(in.SiteURL, "/") + "/sitemap.xml\n"
	robotsPath := "robots.txt"
	if in.Structure != nil && in.Structure.PublicRoot != "" {
		robotsPath = strings.TrimSuffix(in.Structure.PublicRoot, "/") + "/robots.txt"
	}
	files[robotsPath] = robots

	// SEO patch notes as markdown for humans / other agents
	var md strings.Builder
	md.WriteString("# SEO Analyst proposed changes\n\n")
	md.WriteString("Site: " + in.SiteURL + "\n\n")
	if in.ProposedSlug != "" {
		md.WriteString("- Proposed slug: `" + in.ProposedSlug + "`\n")
	}
	for k, v := range in.MetaPatch {
		fmt.Fprintf(&md, "- Meta `%s`: %s\n", k, v)
	}
	md.WriteString("\n## Skills\n\n")
	for _, h := range SkillHints {
		md.WriteString("- " + h + "\n")
	}
	files["SEO_PATCH.md"] = md.String()

	for path, content := range files {
		if err := gh.putFile(ctx, owner, repo, branch, path, content, "seo: update "+path); err != nil {
			return PublishResult{Channel: "git_pr", Success: false, Message: "commit " + path + ": " + err.Error()}
		}
	}

	prClient := githubAdapter.NewPullRequestClient(token)
	pr, err := prClient.CreatePullRequest(ctx, owner, repo,
		"seo: improve meta, slug hints, and sitemap",
		branch, base,
		"Automated SEO Analyst improvements.\n\nIncludes sitemap/robots updates and SEO_PATCH.md with meta/slug proposals.\n\nGenerated by JobShout SEO Analyst.")
	if err != nil {
		return PublishResult{Channel: "git_pr", Success: false, Message: "create PR: " + err.Error(), RemoteRef: branch}
	}
	return PublishResult{
		Channel:   "git_pr",
		Success:   true,
		Message:   "Opened PR with sitemap, robots.txt, and SEO_PATCH.md",
		PRURL:     pr.HTMLURL,
		PRNumber:  pr.Number,
		RemoteRef: branch,
	}
}

type gitHubFiles struct {
	Token string
	HTTP  *http.Client
}

func (g *gitHubFiles) ensureBranch(ctx context.Context, owner, repo, base, branch string) error {
	sha, err := g.refSHA(ctx, owner, repo, "heads/"+base)
	if err != nil {
		// try master
		sha, err = g.refSHA(ctx, owner, repo, "heads/master")
		if err != nil {
			return err
		}
	}
	body, _ := json.Marshal(map[string]string{
		"ref": "refs/heads/" + branch,
		"sha": sha,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("https://api.github.com/repos/%s/%s/git/refs", owner, repo), bytes.NewReader(body))
	if err != nil {
		return err
	}
	g.auth(req)
	resp, err := g.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 && resp.StatusCode != 422 { // 422 = already exists
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

func (g *gitHubFiles) refSHA(ctx context.Context, owner, repo, ref string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("https://api.github.com/repos/%s/%s/git/ref/%s", owner, repo, ref), nil)
	if err != nil {
		return "", err
	}
	g.auth(req)
	resp, err := g.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ref %s: %d %s", ref, resp.StatusCode, string(b))
	}
	var out struct {
		Object struct {
			SHA string `json:"sha"`
		} `json:"object"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return out.Object.SHA, nil
}

func (g *gitHubFiles) putFile(ctx context.Context, owner, repo, branch, path, content, message string) error {
	var sha string
	getURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s?ref=%s", owner, repo, path, url.QueryEscape(branch))
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, getURL, nil)
	g.auth(req)
	if resp, err := g.HTTP.Do(req); err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == 200 {
			var existing struct {
				SHA string `json:"sha"`
			}
			_ = json.NewDecoder(resp.Body).Decode(&existing)
			sha = existing.SHA
		}
	}
	payload := map[string]any{
		"message": message,
		"content": base64.StdEncoding.EncodeToString([]byte(content)),
		"branch":  branch,
	}
	if sha != "" {
		payload["sha"] = sha
	}
	raw, _ := json.Marshal(payload)
	put, err := http.NewRequestWithContext(ctx, http.MethodPut,
		fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s", owner, repo, path), bytes.NewReader(raw))
	if err != nil {
		return err
	}
	g.auth(put)
	resp, err := g.HTTP.Do(put)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

func (g *gitHubFiles) auth(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+g.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if req.Header.Get("Content-Type") == "" && req.Method != http.MethodGet {
		req.Header.Set("Content-Type", "application/json")
	}
}

func parseRepo(raw string) (owner, repo string, err error) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimSuffix(raw, ".git")
	if raw == "" {
		return "", "", fmt.Errorf("git_repo is empty")
	}
	if strings.Contains(raw, "github.com/") {
		u, e := url.Parse(raw)
		if e != nil {
			return "", "", e
		}
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) < 2 {
			return "", "", fmt.Errorf("git_repo URL must be github.com/owner/repo")
		}
		return parts[0], parts[1], nil
	}
	parts := strings.Split(raw, "/")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("git_repo must be owner/repo")
	}
	return parts[0], parts[1], nil
}

func firstEnv(explicit string, keys ...string) string {
	if strings.TrimSpace(explicit) != "" {
		return strings.TrimSpace(explicit)
	}
	for _, k := range keys {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return ""
}
