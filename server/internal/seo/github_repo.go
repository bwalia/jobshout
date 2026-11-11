package seo

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"
)

// RepoClient reads/writes files via the GitHub Contents + Git Trees APIs.
type RepoClient struct {
	Token  string
	HTTP   *http.Client
	APIBase string // override for tests; default https://api.github.com
}

func (c *RepoClient) base() string {
	if c.APIBase != "" {
		return strings.TrimRight(c.APIBase, "/")
	}
	return "https://api.github.com"
}

func (c *RepoClient) http() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 45 * time.Second}
}

// ListPaths returns recursive file paths under the repo at ref.
func (c *RepoClient) ListPaths(ctx context.Context, owner, repo, ref string) ([]string, error) {
	if ref == "" {
		ref = "main"
	}
	url := fmt.Sprintf("%s/repos/%s/%s/git/trees/%s?recursive=1", c.base(), owner, repo, ref)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	c.auth(req)
	resp, err := c.http().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("list tree: %d %s", resp.StatusCode, truncate(string(body), 200))
	}
	var out struct {
		Tree []struct {
			Path string `json:"path"`
			Type string `json:"type"`
		} `json:"tree"`
		Truncated bool `json:"truncated"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	var paths []string
	for _, t := range out.Tree {
		if t.Type == "blob" && t.Path != "" {
			paths = append(paths, t.Path)
		}
	}
	return paths, nil
}

// GetFile returns decoded file content at path/ref.
func (c *RepoClient) GetFile(ctx context.Context, owner, repo, ref, filePath string) (string, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/contents/%s?ref=%s", c.base(), owner, repo, strings.TrimPrefix(filePath, "/"), ref)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	c.auth(req)
	resp, err := c.http().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("get file: %d %s", resp.StatusCode, truncate(string(body), 200))
	}
	var out struct {
		Encoding string `json:"encoding"`
		Content  string `json:"content"`
		Type     string `json:"type"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", err
	}
	if out.Type != "" && out.Type != "file" {
		return "", fmt.Errorf("path is not a file")
	}
	raw := strings.ReplaceAll(out.Content, "\n", "")
	if out.Encoding == "base64" || out.Encoding == "" {
		b, err := base64.StdEncoding.DecodeString(raw)
		if err != nil {
			return "", fmt.Errorf("decode content: %w", err)
		}
		return string(b), nil
	}
	return out.Content, nil
}

func (c *RepoClient) auth(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
}

// MatchGlob filters paths with a simple glob (** and *). Empty glob matches common page types.
func MatchGlob(paths []string, glob string, max int) []string {
	if max <= 0 {
		max = 10
	}
	glob = strings.TrimSpace(glob)
	var out []string
	for _, p := range paths {
		if !isPagePath(p) {
			continue
		}
		if glob == "" || matchSimpleGlob(glob, p) {
			out = append(out, p)
			if len(out) >= max {
				break
			}
		}
	}
	return out
}

func isPagePath(p string) bool {
	lp := strings.ToLower(p)
	switch {
	case strings.HasSuffix(lp, ".html"), strings.HasSuffix(lp, ".htm"),
		strings.HasSuffix(lp, ".md"), strings.HasSuffix(lp, ".mdx"),
		strings.HasSuffix(lp, ".markdown"):
		return true
	default:
		return false
	}
}

// matchSimpleGlob supports *, **, and path.Match semantics loosely.
func matchSimpleGlob(pattern, name string) bool {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" || pattern == "**" || pattern == "**/*" {
		return true
	}
	if ok, err := path.Match(pattern, name); err == nil && ok {
		return true
	}
	if ok, err := path.Match(pattern, path.Base(name)); err == nil && ok {
		return true
	}
	if strings.Contains(pattern, "**") {
		chunks := strings.SplitN(pattern, "**", 2)
		pre := strings.TrimSuffix(chunks[0], "/")
		suf := strings.TrimPrefix(chunks[1], "/")
		if pre != "" && !(name == pre || strings.HasPrefix(name, pre+"/")) {
			return false
		}
		rest := name
		if pre != "" {
			rest = strings.TrimPrefix(name, pre)
			rest = strings.TrimPrefix(rest, "/")
		}
		if suf == "" || suf == "*" || suf == "/*" {
			return true
		}
		suf = strings.TrimPrefix(suf, "/")
		if ok, _ := path.Match(suf, rest); ok {
			return true
		}
		if ok, _ := path.Match(suf, path.Base(name)); ok {
			return true
		}
		parts := strings.Split(rest, "/")
		for i := range parts {
			sub := strings.Join(parts[i:], "/")
			if ok, _ := path.Match(suf, sub); ok {
				return true
			}
		}
		return false
	}
	// prefix/* style without **
	if strings.Contains(pattern, "/") {
		if ok, _ := path.Match(pattern, name); ok {
			return true
		}
	}
	return false
}

// ParseURLs splits a textarea of URLs (newline / comma).
func ParseURLs(raw string) []string {
	raw = strings.ReplaceAll(raw, ",", "\n")
	var out []string
	seen := map[string]bool{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !strings.Contains(line, "://") && strings.Contains(line, ".") {
			line = "https://" + line
		}
		if seen[line] {
			continue
		}
		seen[line] = true
		out = append(out, line)
	}
	return out
}
