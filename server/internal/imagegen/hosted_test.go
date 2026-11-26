package imagegen

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func sizedPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func fastPolling(t *testing.T) {
	t.Helper()
	oe, ot, or := pollEvery, pollTimeout, defaultRetry
	pollEvery, pollTimeout = time.Millisecond, 2*time.Second
	defaultRetry = retryPolicy{attempts: 3, base: time.Millisecond}
	t.Cleanup(func() { pollEvery, pollTimeout, defaultRetry = oe, ot, or })
}

func TestFlux_BFL_SubmitPollDownload(t *testing.T) {
	fastPolling(t)
	img := sizedPNG(t, 1200, 640)
	var polls int32
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/test-model":
			if r.Header.Get("x-key") != "k" {
				t.Errorf("x-key = %q", r.Header.Get("x-key"))
			}
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["prompt"] != "a hill" || body["width"].(float64) != 1200 {
				t.Errorf("body = %v", body)
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "j1", "polling_url": srv.URL + "/poll?id=j1"})
		case r.URL.Path == "/poll":
			if atomic.AddInt32(&polls, 1) < 2 {
				_, _ = w.Write([]byte(`{"status":"Pending"}`))
				return
			}
			_, _ = w.Write([]byte(`{"status":"Ready","result":{"sample":"` + srv.URL + `/img.png"}}`))
		case r.URL.Path == "/img.png":
			if r.Header.Get("x-key") != "" {
				t.Error("API key must not be sent to the image host")
			}
			_, _ = w.Write(img)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := NewFluxClient("bfl", srv.URL, "k", "test-model")
	resp, err := c.Generate(context.Background(), GenerateRequest{Prompt: "a hill", Width: 1200, Height: 630, Seed: -1})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Provider != ProviderFlux || resp.Model != "test-model" || resp.Width != 1200 {
		t.Errorf("resp = %+v", resp)
	}
	if polls < 2 {
		t.Errorf("polled %d times, want >= 2", polls)
	}
}

func TestFlux_BFL_ModeratedAndRateLimited(t *testing.T) {
	fastPolling(t)
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "j", "polling_url": srv.URL + "/poll"})
			return
		}
		_, _ = w.Write([]byte(`{"status":"Request Moderated"}`))
	}))
	defer srv.Close()
	_, err := NewFluxClient("bfl", srv.URL, "k", "m").Generate(context.Background(), GenerateRequest{Prompt: "x"})
	if !IsModerated(err) {
		t.Errorf("err = %v, want moderated", err)
	}

	rl := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(429) }))
	defer rl.Close()
	_, err = NewFluxClient("bfl", rl.URL, "k", "m").Generate(context.Background(), GenerateRequest{Prompt: "x"})
	if !IsRateLimited(err) {
		t.Errorf("err = %v, want rate limited", err)
	}
}

func TestFlux_NotConfigured(t *testing.T) {
	if _, err := NewFluxClient("bfl", "", "", "m").Generate(context.Background(), GenerateRequest{Prompt: "x"}); err == nil {
		t.Error("want error without a key")
	}
	if _, err := NewFluxClient("bfl", "", "k", "").Generate(context.Background(), GenerateRequest{Prompt: "x"}); err == nil ||
		!strings.Contains(err.Error(), "FLUX_MODEL") {
		t.Errorf("err = %v, want a FLUX_MODEL hint", err)
	}
}

func TestFlux_Fal_QueueFlow(t *testing.T) {
	fastPolling(t)
	img := sizedPNG(t, 1200, 640)
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/media.png" && r.Header.Get("Authorization") != "Key fk" {
			t.Errorf("auth = %q on %s", r.Header.Get("Authorization"), r.URL.Path)
		}
		switch r.URL.Path {
		case "/fal-ai/flux-pro/v1.1":
			var in falInput
			_ = json.NewDecoder(r.Body).Decode(&in)
			if in.ImageSize.Width != 1200 || in.Prompt != "a hill" {
				t.Errorf("input = %+v", in)
			}
			_ = json.NewEncoder(w).Encode(falSubmitResponse{
				RequestID: "r1", StatusURL: srv.URL + "/fal-ai/flux-pro/requests/r1/status",
				ResponseURL: srv.URL + "/fal-ai/flux-pro/requests/r1",
			})
		case "/fal-ai/flux-pro/requests/r1/status":
			_, _ = w.Write([]byte(`{"status":"COMPLETED"}`))
		case "/fal-ai/flux-pro/requests/r1":
			_, _ = w.Write([]byte(`{"images":[{"url":"` + srv.URL + `/media.png"}]}`))
		case "/media.png":
			_, _ = w.Write(img)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := NewFluxClient("fal", srv.URL, "fk", "fal-ai/flux-pro/v1.1")
	resp, err := c.Generate(context.Background(), GenerateRequest{Prompt: "a hill", Width: 1200, Height: 630, Seed: -1})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Width != 1200 || resp.Provider != ProviderFlux {
		t.Errorf("resp = %+v", resp)
	}
}

func TestFlux_Fal_ContentPolicyIsModerated(t *testing.T) {
	fastPolling(t)
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_ = json.NewEncoder(w).Encode(falSubmitResponse{RequestID: "r", StatusURL: srv.URL + "/s", ResponseURL: srv.URL + "/r"})
			return
		}
		_, _ = w.Write([]byte(`{"status":"COMPLETED","error":"blocked","error_type":"content_policy_violation"}`))
	}))
	defer srv.Close()
	_, err := NewFluxClient("fal", srv.URL, "k", "m").Generate(context.Background(), GenerateRequest{Prompt: "x"})
	if !IsModerated(err) {
		t.Errorf("err = %v, want moderated", err)
	}
}

func TestFlux_PollTimeout(t *testing.T) {
	fastPolling(t)
	pollTimeout = 30 * time.Millisecond
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "j", "polling_url": srv.URL + "/p"})
			return
		}
		_, _ = w.Write([]byte(`{"status":"Pending"}`))
	}))
	defer srv.Close()
	_, err := NewFluxClient("bfl", srv.URL, "k", "m").Generate(context.Background(), GenerateRequest{Prompt: "x"})
	if err == nil || !strings.Contains(err.Error(), "did not finish") {
		t.Errorf("err = %v, want timeout", err)
	}
}

func TestMidjourney_Bridge(t *testing.T) {
	fastPolling(t)
	img := sizedPNG(t, 1024, 576)
	for name, handler := range map[string]http.HandlerFunc{
		"raw image": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(img)
		},
		"base64 json": func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]string{"image_base64": base64.StdEncoding.EncodeToString(img)})
		},
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(handler)
			defer srv.Close()
			resp, err := NewMidjourneyClient(srv.URL, "mk", "v-test", time.Second).
				Generate(context.Background(), GenerateRequest{Prompt: "a hill"})
			if err != nil {
				t.Fatal(err)
			}
			if resp.Provider != ProviderMidjourney || resp.Model != "v-test" {
				t.Errorf("resp = %+v", resp)
			}
		})
	}
}

func TestMidjourney_BridgeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"error":"bridge is down"}`))
	}))
	defer srv.Close()
	_, err := NewMidjourneyClient(srv.URL, "", "", time.Second).Generate(context.Background(), GenerateRequest{Prompt: "x"})
	if err == nil || !strings.Contains(err.Error(), "bridge is down") {
		t.Errorf("err = %v", err)
	}
}

func TestFinishImage_Validation(t *testing.T) {
	now := time.Now()
	if _, err := finishImage("p", "m", []byte("<html>nope</html>"), 0, 0, now); err == nil {
		t.Error("non-image bytes accepted")
	}
	if _, err := finishImage("p", "m", sizedPNG(t, 64, 64), 0, 0, now); err == nil ||
		!strings.Contains(err.Error(), "minimum") {
		t.Errorf("tiny image: err = %v", err)
	}
	if _, err := finishImage("p", "m", nil, 0, 0, now); err == nil {
		t.Error("empty accepted")
	}
}

// scriptClient lets router tests script each provider's outcome.
type scriptClient struct {
	name  string
	calls []GenerateRequest
	errs  []error // consumed one per call; nil entry or exhausted = success
}

func (s *scriptClient) Provider() string { return s.name }
func (s *scriptClient) ListModels(context.Context) ([]ModelInfo, error) {
	return nil, nil
}
func (s *scriptClient) Generate(_ context.Context, r GenerateRequest) (*GenerateResponse, error) {
	i := len(s.calls)
	s.calls = append(s.calls, r)
	if i < len(s.errs) && s.errs[i] != nil {
		return nil, s.errs[i]
	}
	return &GenerateResponse{PNG: []byte("x"), Provider: s.name}, nil
}

func TestRouter_FallsBackOnRateLimit(t *testing.T) {
	gem := &scriptClient{name: ProviderGemini, errs: []error{&ProviderError{Provider: "gemini", Kind: FailureRateLimited, Msg: "slow"}}}
	flux := &scriptClient{name: ProviderFlux}
	r := NewTestRouter(ProviderGemini, map[string]Client{ProviderGemini: gem, ProviderFlux: flux})
	r.fallbackOrder = []string{ProviderGemini, ProviderFlux}

	resp, err := r.Generate(context.Background(), "", GenerateRequest{Prompt: "a hill", Model: "gemini-x"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Provider != ProviderFlux {
		t.Errorf("served by %s, want flux", resp.Provider)
	}
	if flux.calls[0].Model != "" {
		t.Errorf("fallback inherited model %q", flux.calls[0].Model)
	}
}

func TestRouter_ModerationRetriesSimplified(t *testing.T) {
	mod := &ProviderError{Provider: "flux", Kind: FailureModerated, Msg: "no"}
	flux := &scriptClient{name: ProviderFlux, errs: []error{mod}}
	r := NewTestRouter(ProviderFlux, map[string]Client{ProviderFlux: flux})

	if _, err := r.Generate(context.Background(), ProviderFlux, GenerateRequest{Prompt: "a very specific prompt", NegativePrompt: "x"}); err != nil {
		t.Fatal(err)
	}
	if len(flux.calls) != 2 {
		t.Fatalf("calls = %d, want 2", len(flux.calls))
	}
	if !strings.Contains(flux.calls[1].Prompt, "family-friendly") || flux.calls[1].NegativePrompt != "" {
		t.Errorf("retry request = %+v", flux.calls[1])
	}
}

func TestRouter_PersistentModerationGivesFriendlyError(t *testing.T) {
	mod := &ProviderError{Provider: "flux", Kind: FailureModerated, Msg: "no"}
	flux := &scriptClient{name: ProviderFlux, errs: []error{mod, mod}}
	r := NewTestRouter(ProviderFlux, map[string]Client{ProviderFlux: flux})
	_, err := r.Generate(context.Background(), ProviderFlux, GenerateRequest{Prompt: "p"})
	if err == nil || !strings.Contains(err.Error(), "try rewording") || !IsModerated(err) {
		t.Errorf("err = %v", err)
	}
}

func TestRouter_ContextCancelStopsFallback(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	a := &scriptClient{name: ProviderGemini, errs: []error{errors.New("boom")}}
	b := &scriptClient{name: ProviderFlux}
	r := NewTestRouter(ProviderGemini, map[string]Client{ProviderGemini: a, ProviderFlux: b})
	r.fallbackOrder = []string{ProviderFlux}
	if _, err := r.Generate(ctx, ProviderGemini, GenerateRequest{Prompt: "p"}); err == nil {
		t.Error("want error")
	}
	if len(b.calls) != 0 {
		t.Error("fell back after cancellation")
	}
}

func TestSimplifyPrompt_Bounded(t *testing.T) {
	got := SimplifyPrompt(strings.Repeat("word ", 200))
	if len(got) > 400 || !strings.Contains(got, "no text") {
		t.Errorf("len=%d %q", len(got), got)
	}
}
