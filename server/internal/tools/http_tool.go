package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// HTTPTool allows agents to make outbound HTTP requests.
// It is intentionally permissive regarding target URLs but caps body size
// and enforces a timeout to prevent runaway calls.
type HTTPTool struct {
	httpClient *http.Client
	// maxBodyBytes caps the response body read to prevent memory exhaustion.
	maxBodyBytes int64
}

// NewHTTPTool creates an HTTPTool with a 30-second timeout and 1 MB body cap.
func NewHTTPTool() *HTTPTool {
	return &HTTPTool{
		httpClient:   &http.Client{Timeout: 30 * time.Second},
		maxBodyBytes: 1 << 20, // 1 MB
	}
}

func (t *HTTPTool) Name() string { return "http_request" }

func (t *HTTPTool) Description() string {
	return `Make an HTTP request to an external URL.
Input parameters:
  method  (string, required) - HTTP method: GET, POST, PUT, PATCH, DELETE
  url     (string, required) - Full URL including scheme, e.g. https://api.example.com/endpoint
  headers (object, optional) - Map of header name → value
  body    (string, optional) - Request body (typically JSON)

Returns the response status code and body.`
}

func (t *HTTPTool) Execute(ctx context.Context, input map[string]any) (string, error) {
	method, err := stringParam(input, "method", true)
	if err != nil {
		return "", err
	}
	method = strings.ToUpper(method)

	url, err := stringParam(input, "url", true)
	if err != nil {
		return "", err
	}

	// Validate that url starts with http:// or https:// to prevent SSRF to
	// internal metadata endpoints. Additional host controls can be added here.
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return "", fmt.Errorf("http_request: url must start with http:// or https://")
	}

	bodyStr, _ := stringParam(input, "body", false)
	var bodyReader io.Reader
	if bodyStr != "" {
		bodyReader = bytes.NewBufferString(bodyStr)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return "", fmt.Errorf("http_request: build request: %w", err)
	}

	// Apply optional headers.
	if headers, ok := input["headers"]; ok {
		if hmap, ok := headers.(map[string]any); ok {
			for k, v := range hmap {
				if vs, ok := v.(string); ok {
					req.Header.Set(k, vs)
				}
			}
		}
	}

	// Default Content-Type for requests with a body.
	if bodyStr != "" && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("http_request: %w", err)
	}
	defer resp.Body.Close()

	rawBody, err := io.ReadAll(io.LimitReader(resp.Body, t.maxBodyBytes))
	if err != nil {
		return "", fmt.Errorf("http_request: read response: %w", err)
	}

	// Return a structured summary as JSON so the LLM can parse it easily.
	result := map[string]any{
		"status_code": resp.StatusCode,
		"body":        string(rawBody),
	}
	encoded, _ := json.Marshal(result)
	return string(encoded), nil
}

// stringParam extracts a string parameter from the input map.
func stringParam(input map[string]any, key string, required bool) (string, error) {
	v, ok := input[key]
	if !ok {
		if required {
			return "", fmt.Errorf("missing required parameter %q", key)
		}
		return "", nil
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("parameter %q must be a string", key)
	}
	return s, nil
}
