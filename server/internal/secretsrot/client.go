package secretsrot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client talks Vault-compatible KV v2 / transit / sys APIs (HC Vault + WSLVault).
type Client struct {
	cfg  Config
	http *http.Client
}

// NewClient constructs an HTTP Vault client.
func NewClient(cfg Config) *Client {
	t := cfg.Timeout
	if t <= 0 {
		t = 30 * time.Second
	}
	return &Client{
		cfg:  cfg,
		http: &http.Client{Timeout: t},
	}
}

func (c *Client) Enabled() bool { return c.cfg.Enabled() }
func (c *Client) Addr() string  { return c.cfg.Addr }
func (c *Client) UILogin() string {
	if c.cfg.UILogin != "" {
		return c.cfg.UILogin
	}
	return DefaultUILoginURL
}
func (c *Client) DocsURL() string {
	if c.cfg.DocsURL != "" {
		return c.cfg.DocsURL
	}
	return DefaultDocsURL
}

// Health probes sys/health (auth may be optional).
func (c *Client) Health(ctx context.Context) (map[string]any, error) {
	var out map[string]any
	code, err := c.do(ctx, http.MethodGet, "/v1/sys/health", nil, &out)
	if err != nil && code == 0 {
		return nil, err
	}
	if out == nil {
		out = map[string]any{}
	}
	out["http_status"] = code
	return out, nil
}

// DetectProvider guesses wslvault vs hashicorp from addr / health body.
func DetectProvider(addr string, health map[string]any) string {
	a := strings.ToLower(addr)
	if strings.Contains(a, "workstation.co.uk") || strings.Contains(a, "wslvault") {
		return "wslvault"
	}
	if health != nil {
		if _, ok := health["replication_lag"]; ok {
			return "wslvault"
		}
		if _, ok := health["initialized"]; ok {
			return "hashicorp"
		}
	}
	if strings.Contains(a, "vault.hashicorp") {
		return "hashicorp"
	}
	return "hashicorp" // Vault-compatible default
}

// SecretMeta is version metadata without values.
type SecretMeta struct {
	Version     int
	CreatedTime string
	Keys        []string
	Destroyed   bool
	Versions    map[string]any
}

// ReadKV2 reads the latest (or specific) KV v2 secret; returns data map + version.
func (c *Client) ReadKV2(ctx context.Context, mount, path string, version int) (map[string]any, SecretMeta, error) {
	mount = strings.Trim(mount, "/")
	path = strings.Trim(path, "/")
	q := ""
	if version > 0 {
		q = fmt.Sprintf("?version=%d", version)
	}
	var raw map[string]any
	code, err := c.do(ctx, http.MethodGet, fmt.Sprintf("/v1/%s/data/%s%s", mount, path, q), nil, &raw)
	if err != nil {
		return nil, SecretMeta{}, err
	}
	if code == 404 {
		return nil, SecretMeta{}, fmt.Errorf("secret not found at %s/%s", mount, path)
	}
	if code >= 300 {
		return nil, SecretMeta{}, fmt.Errorf("read failed: HTTP %d", code)
	}
	data, meta := parseKV2(raw)
	return data, meta, nil
}

// WriteKV2 writes a new version (check-and-set optional via cas).
func (c *Client) WriteKV2(ctx context.Context, mount, path string, data map[string]any, cas int) (SecretMeta, error) {
	mount = strings.Trim(mount, "/")
	path = strings.Trim(path, "/")
	body := map[string]any{"data": data}
	if cas > 0 {
		body["options"] = map[string]any{"cas": cas}
	}
	var raw map[string]any
	code, err := c.do(ctx, http.MethodPost, fmt.Sprintf("/v1/%s/data/%s", mount, path), body, &raw)
	if err != nil {
		return SecretMeta{}, err
	}
	if code >= 300 {
		return SecretMeta{}, fmt.Errorf("write failed: HTTP %d", code)
	}
	_, meta := parseKV2(raw)
	if meta.Version == 0 {
		// Some servers return version only in metadata nested under data.
		if m, ok := raw["data"].(map[string]any); ok {
			if v, ok := asInt(m["version"]); ok {
				meta.Version = v
			}
		}
	}
	return meta, nil
}

// SoftDeleteVersions marks versions deleted (still recoverable until destroy).
func (c *Client) SoftDeleteVersions(ctx context.Context, mount, path string, versions []int) error {
	mount = strings.Trim(mount, "/")
	path = strings.Trim(path, "/")
	body := map[string]any{"versions": versions}
	code, err := c.do(ctx, http.MethodPost, fmt.Sprintf("/v1/%s/delete/%s", mount, path), body, nil)
	if err != nil {
		return err
	}
	if code >= 300 {
		return fmt.Errorf("soft-delete failed: HTTP %d", code)
	}
	return nil
}

// UndeleteVersions restores soft-deleted versions (rollback helper).
func (c *Client) UndeleteVersions(ctx context.Context, mount, path string, versions []int) error {
	mount = strings.Trim(mount, "/")
	path = strings.Trim(path, "/")
	body := map[string]any{"versions": versions}
	code, err := c.do(ctx, http.MethodPost, fmt.Sprintf("/v1/%s/undelete/%s", mount, path), body, nil)
	if err != nil {
		return err
	}
	if code >= 300 {
		return fmt.Errorf("undelete failed: HTTP %d", code)
	}
	return nil
}

// MetadataKV2 fetches version table without secret values.
func (c *Client) MetadataKV2(ctx context.Context, mount, path string) (SecretMeta, error) {
	mount = strings.Trim(mount, "/")
	path = strings.Trim(path, "/")
	var raw map[string]any
	code, err := c.do(ctx, http.MethodGet, fmt.Sprintf("/v1/%s/metadata/%s", mount, path), nil, &raw)
	if err != nil {
		return SecretMeta{}, err
	}
	if code == 404 {
		// WSLVault serves KV v2 data but not the metadata endpoint, so a 404
		// here does not mean the secret is missing. The data read carries the
		// current version too; its values are dropped unread.
		_, meta, rerr := c.ReadKV2(ctx, mount, path, 0)
		if rerr != nil {
			return SecretMeta{}, fmt.Errorf("metadata not found at %s/%s", mount, path)
		}
		return meta, nil
	}
	if code >= 300 {
		return SecretMeta{}, fmt.Errorf("metadata failed: HTTP %d", code)
	}
	meta := SecretMeta{}
	if d, ok := raw["data"].(map[string]any); ok {
		if v, ok := asInt(d["current_version"]); ok {
			meta.Version = v
		}
		if vs, ok := d["versions"].(map[string]any); ok {
			meta.Versions = vs
		}
	}
	return meta, nil
}

// RotateTransitKey rotates a transit key (old ciphertext remains decryptable).
func (c *Client) RotateTransitKey(ctx context.Context, mount, name string) error {
	mount = strings.Trim(mount, "/")
	name = strings.Trim(name, "/")
	code, err := c.do(ctx, http.MethodPost, fmt.Sprintf("/v1/%s/keys/%s/rotate", mount, name), nil, nil)
	if err != nil {
		return err
	}
	if code >= 300 {
		return fmt.Errorf("transit rotate failed: HTTP %d", code)
	}
	return nil
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) (int, error) {
	if c.cfg.Addr == "" {
		return 0, fmt.Errorf("vault address not configured")
	}
	u, err := url.Parse(c.cfg.Addr + path)
	if err != nil {
		return 0, err
	}
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), rdr)
	if err != nil {
		return 0, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.cfg.Token != "" {
		req.Header.Set("X-Vault-Token", c.cfg.Token)
	}
	if c.cfg.Namespace != "" {
		req.Header.Set("X-Vault-Namespace", c.cfg.Namespace)
		req.Header.Set("X-Tenant-Id", c.cfg.Namespace)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if out != nil && len(raw) > 0 {
		_ = json.Unmarshal(raw, out)
	}
	if resp.StatusCode >= 400 && resp.StatusCode != 404 {
		msg := strings.TrimSpace(string(raw))
		if len(msg) > 240 {
			msg = msg[:240] + "…"
		}
		return resp.StatusCode, fmt.Errorf("vault HTTP %d: %s", resp.StatusCode, msg)
	}
	return resp.StatusCode, nil
}

func parseKV2(raw map[string]any) (map[string]any, SecretMeta) {
	meta := SecretMeta{}
	data := map[string]any{}
	inner, ok := raw["data"].(map[string]any)
	if !ok {
		return data, meta
	}
	if d, ok := inner["data"].(map[string]any); ok {
		data = d
		for k := range d {
			meta.Keys = append(meta.Keys, k)
		}
	}
	if m, ok := inner["metadata"].(map[string]any); ok {
		if v, ok := asInt(m["version"]); ok {
			meta.Version = v
		}
		if t, ok := m["created_time"].(string); ok {
			meta.CreatedTime = t
		}
		if dest, ok := m["destroyed"].(bool); ok {
			meta.Destroyed = dest
		}
	}
	if meta.Version == 0 {
		if v, ok := asInt(inner["version"]); ok {
			meta.Version = v
		}
	}
	return data, meta
}

func asInt(v any) (int, bool) {
	switch t := v.(type) {
	case float64:
		return int(t), true
	case int:
		return t, true
	case json.Number:
		i, err := t.Int64()
		return int(i), err == nil
	default:
		return 0, false
	}
}
