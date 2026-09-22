package secretsrot

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"golang.org/x/crypto/nacl/box"
)

var (
	githubRepoRe       = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
	githubSecretNameRe = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)
	githubEnvRe        = regexp.MustCompile(`^[A-Za-z0-9_. -]{1,255}$`)
)

// GitHubSecretName is the default Actions secret name for a vault key.
func GitHubSecretName(key string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(key) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	return b.String()
}

func validGitHubSecretName(name string) error {
	if !githubSecretNameRe.MatchString(name) {
		return fmt.Errorf("invalid GitHub secret name %q (A-Z, 0-9, _; not starting with a digit)", name)
	}
	if strings.HasPrefix(name, "GITHUB_") {
		return fmt.Errorf("invalid GitHub secret name %q (GITHUB_ prefix is reserved)", name)
	}
	return nil
}

// GitHubClient writes Actions secrets (repository or environment scope).
type GitHubClient struct {
	api   string
	token string
	http  *http.Client
}

// NewGitHubClient builds a client for the dedicated rotation token.
func NewGitHubClient(api, token string, timeout time.Duration) *GitHubClient {
	if api == "" {
		api = DefaultGitHubAPI
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &GitHubClient{api: strings.TrimRight(api, "/"), token: token, http: &http.Client{Timeout: timeout}}
}

func (g *GitHubClient) do(ctx context.Context, method, path string, body any, out any) (int, error) {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, g.api+path, rdr)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("Authorization", "Bearer "+g.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := g.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		var e struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(raw, &e)
		if e.Message == "" {
			e.Message = http.StatusText(resp.StatusCode)
		}
		return resp.StatusCode, fmt.Errorf("github HTTP %d: %s", resp.StatusCode, e.Message)
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return resp.StatusCode, fmt.Errorf("github decode: %w", err)
		}
	}
	return resp.StatusCode, nil
}

// secretsBase returns the path prefix for the secrets collection. Environment
// secrets use GitHub's repository-id endpoint shape
// (/repositories/{id}/environments/{env}/secrets).
func (g *GitHubClient) secretsBase(ctx context.Context, repo, env string) (string, error) {
	if env == "" {
		return "/repos/" + repo + "/actions/secrets", nil
	}
	var r struct {
		ID int64 `json:"id"`
	}
	if _, err := g.do(ctx, http.MethodGet, "/repos/"+repo, nil, &r); err != nil {
		return "", fmt.Errorf("resolve repository id: %w", err)
	}
	if r.ID == 0 {
		return "", fmt.Errorf("resolve repository id: empty id for %s", repo)
	}
	return fmt.Sprintf("/repositories/%d/environments/%s/secrets", r.ID, url.PathEscape(env)), nil
}

// SealSecret encrypts value for GitHub with a libsodium sealed box.
func SealSecret(publicKeyB64, value string) (string, error) {
	pk, err := base64.StdEncoding.DecodeString(publicKeyB64)
	if err != nil || len(pk) != 32 {
		return "", fmt.Errorf("invalid GitHub public key")
	}
	var key [32]byte
	copy(key[:], pk)
	sealed, err := box.SealAnonymous(nil, []byte(value), &key, rand.Reader)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// PutSecrets encrypts and writes each name→value. Returns names written.
func (g *GitHubClient) PutSecrets(ctx context.Context, repo, env string, values map[string]string, order []string) ([]string, error) {
	base, err := g.secretsBase(ctx, repo, env)
	if err != nil {
		return nil, err
	}
	var pk struct {
		KeyID string `json:"key_id"`
		Key   string `json:"key"`
	}
	if _, err := g.do(ctx, http.MethodGet, base+"/public-key", nil, &pk); err != nil {
		return nil, fmt.Errorf("public key: %w", err)
	}
	var written []string
	for _, name := range order {
		enc, err := SealSecret(pk.Key, values[name])
		if err != nil {
			return written, err
		}
		body := map[string]string{"encrypted_value": enc, "key_id": pk.KeyID}
		if _, err := g.do(ctx, http.MethodPut, base+"/"+url.PathEscape(name), body, nil); err != nil {
			return written, fmt.Errorf("put %s: %w", name, err)
		}
		written = append(written, name)
	}
	return written, nil
}
