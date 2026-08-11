package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "test-shared-secret"

// parseToken verifies a minted token against the shared secret and returns its
// claims — i.e. it does what the gateway does.
func parseToken(t *testing.T, raw string) jwt.MapClaims {
	t.Helper()
	claims := jwt.MapClaims{}
	tok, err := jwt.ParseWithClaims(raw, claims, func(tk *jwt.Token) (any, error) {
		if _, ok := tk.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return []byte(testSecret), nil
	})
	if err != nil {
		t.Fatalf("token did not verify against the shared secret: %v", err)
	}
	if !tok.Valid {
		t.Fatal("token parsed but is not valid")
	}
	return claims
}

func TestOllamaAuth_TokenClaims(t *testing.T) {
	auth := newOllamaAuth(testSecret)
	req := httptest.NewRequest(http.MethodPost, "http://gateway/api/chat", nil)

	before := time.Now()
	if err := auth.apply(req); err != nil {
		t.Fatalf("apply: %v", err)
	}

	// The gateway wants the bare token, not "Bearer <token>".
	raw := req.Header.Get("x-api-key")
	if raw == "" {
		t.Fatal("x-api-key header was not set")
	}
	if got := req.Header.Get("Authorization"); got != "" {
		t.Errorf("Authorization header should not be used, got %q", got)
	}
	if len(raw) > 7 && raw[:7] == "Bearer " {
		t.Error("token must not carry a Bearer prefix")
	}

	claims := parseToken(t, raw)

	if claims["app"] != AppName {
		t.Errorf("app claim = %v, want %q", claims["app"], AppName)
	}

	iat, ok := claims["iat"].(float64)
	if !ok {
		t.Fatalf("iat claim missing or not numeric: %v", claims["iat"])
	}
	exp, ok := claims["exp"].(float64)
	if !ok {
		t.Fatalf("exp claim missing or not numeric: %v", claims["exp"])
	}

	if int64(iat) < before.Unix()-1 || int64(iat) > time.Now().Unix()+1 {
		t.Errorf("iat %v is not around now (%v)", int64(iat), before.Unix())
	}
	if ttl := time.Duration(exp-iat) * time.Second; ttl != ollamaTokenTTL {
		t.Errorf("token TTL = %v, want %v", ttl, ollamaTokenTTL)
	}
}

// Tokens are minted per request and never reused, so two requests must not
// carry the same credential.
func TestOllamaAuth_MintsAFreshTokenPerRequest(t *testing.T) {
	auth := newOllamaAuth(testSecret)

	first := httptest.NewRequest(http.MethodPost, "http://gateway/api/chat", nil)
	if err := auth.apply(first); err != nil {
		t.Fatalf("apply: %v", err)
	}
	// The claims are second-resolution, so without a gap two tokens would be
	// byte-identical purely by coincidence of timing.
	time.Sleep(1100 * time.Millisecond)
	second := httptest.NewRequest(http.MethodPost, "http://gateway/api/chat", nil)
	if err := auth.apply(second); err != nil {
		t.Fatalf("apply: %v", err)
	}

	if first.Header.Get("x-api-key") == second.Header.Get("x-api-key") {
		t.Error("the same token was reused across two requests")
	}
}

// No secret means no gateway: a plain local Ollama must keep working, unsigned.
func TestOllamaAuth_NoSecretSendsNoHeader(t *testing.T) {
	auth := newOllamaAuth("")
	if auth != nil {
		t.Fatal("an empty secret should produce a nil authenticator")
	}
	if auth.enabled() {
		t.Error("a nil authenticator must report itself disabled")
	}

	req := httptest.NewRequest(http.MethodPost, "http://localhost:11434/api/chat", nil)
	if err := auth.apply(req); err != nil {
		t.Fatalf("apply on nil authenticator: %v", err)
	}
	if got := req.Header.Get("x-api-key"); got != "" {
		t.Errorf("unsigned request carried x-api-key = %q", got)
	}
}

// End to end through the client: the gateway sees a verifiable token.
func TestOllamaClient_SignsChatRequests(t *testing.T) {
	var seen string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("x-api-key")
		_ = json.NewEncoder(w).Encode(ollamaChatResponse{
			Model:   "qwen3-coder:30b",
			Message: ollamaMessage{Role: "assistant", Content: "ok"},
			Done:    true,
		})
	}))
	defer srv.Close()

	c := NewOllamaClientWithAuth(srv.URL, "qwen3-coder:30b", testSecret, 0)
	if !c.UsesGateway() {
		t.Error("client should report gateway mode when a secret is set")
	}

	resp, err := c.Generate(context.Background(), GenerateRequest{
		Messages: []Message{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if resp.Content != "ok" {
		t.Errorf("content = %q", resp.Content)
	}
	if seen == "" {
		t.Fatal("gateway received no x-api-key header")
	}
	if claims := parseToken(t, seen); claims["app"] != AppName {
		t.Errorf("app claim = %v", claims["app"])
	}
}

// A rejection must surface as a clear, terminal error naming the cause —
// retrying with the same secret would fail identically.
func TestOllamaClient_AuthRejectionIsExplicit(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		var attempts int
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			attempts++
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"error":"invalid signature"}`))
		}))

		c := NewOllamaClientWithAuth(srv.URL, "qwen3-coder:30b", testSecret, 0)
		_, err := c.Generate(context.Background(), GenerateRequest{
			Messages: []Message{{Role: "user", Content: "hi"}},
		})
		srv.Close()

		if err == nil {
			t.Fatalf("status %d: expected an error", status)
		}
		if attempts != 1 {
			t.Errorf("status %d: made %d attempts, want exactly 1 (no retry)", status, attempts)
		}
		for _, want := range []string{"signature", "OLLAMA_JWT_SECRET"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("status %d: error %q should mention %q", status, err, want)
			}
		}
	}
}

func TestOllamaEmbedder_SignsRequests(t *testing.T) {
	var seen string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("x-api-key")
		_ = json.NewEncoder(w).Encode(ollamaEmbeddingResponse{Embedding: []float32{0.1, 0.2}})
	}))
	defer srv.Close()

	e := NewOllamaEmbedderWithAuth(srv.URL, "nomic-embed-text", 2, testSecret)
	if _, err := e.Embed(context.Background(), []string{"hello"}); err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if seen == "" {
		t.Fatal("gateway received no x-api-key header on the embedding call")
	}
	parseToken(t, seen)
}
