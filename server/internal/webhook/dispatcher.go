// Package webhook delivers org events to org-configured HTTP endpoints.
//
// Each event is written as one delivery row per subscribed endpoint (the
// delivery log), attempted at once in the background, and retried with
// backoff by a polling worker until it succeeds or runs out of attempts.
// Deliveries are leased in the database (FOR UPDATE SKIP LOCKED), so several
// replicas can run the worker and a crash mid-attempt is retried after the
// lease. Receivers must therefore tolerate duplicates: the envelope "id"
// (also X-JobShout-Event-Id) is stable across retries and endpoints.
//
// Signature: X-JobShout-Signature: sha256=<hex HMAC-SHA256(secret,
// "<X-JobShout-Timestamp>.<raw body>")>. Receivers should recompute it with
// the endpoint secret, compare in constant time, and reject stale timestamps.
package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

// Header names sent with every delivery.
const (
	HeaderEvent     = "X-JobShout-Event"
	HeaderEventID   = "X-JobShout-Event-Id"
	HeaderDelivery  = "X-JobShout-Delivery"
	HeaderTimestamp = "X-JobShout-Timestamp"
	HeaderSignature = "X-JobShout-Signature"
)

// Emitter is what event sources call. Emit never fails the caller.
type Emitter interface {
	Emit(ctx context.Context, orgID uuid.UUID, eventType string, data any)
}

// Envelope is the JSON body of every delivery.
type Envelope struct {
	ID        uuid.UUID `json:"id"`
	Type      string    `json:"type"`
	CreatedAt time.Time `json:"created_at"`
	OrgID     uuid.UUID `json:"org_id"`
	Data      any       `json:"data"`
}

// Options tune a Dispatcher. Zero values are sensible.
type Options struct {
	// Backoff is the wait before each retry; its length+1 is the attempt cap.
	// Default 30s, 2m, 10m, 30m, 2h (6 attempts over ~2h45m).
	Backoff []time.Duration
	// Lease is how long a claimed delivery is reserved for one attempt.
	Lease time.Duration
	// Timeout bounds one HTTP attempt. Default 10s.
	Timeout time.Duration
	// Sync attempts inline in Emit instead of in a goroutine (tests).
	Sync   bool
	Client *http.Client
	Logger *zap.Logger
}

// Dispatcher writes and delivers webhook events.
type Dispatcher struct {
	repo repository.WebhookRepository
	opts Options
	now  func() time.Time
}

// NewDispatcher builds a Dispatcher.
func NewDispatcher(repo repository.WebhookRepository, opts Options) *Dispatcher {
	if len(opts.Backoff) == 0 {
		opts.Backoff = []time.Duration{30 * time.Second, 2 * time.Minute, 10 * time.Minute, 30 * time.Minute, 2 * time.Hour}
	}
	if opts.Lease <= 0 {
		opts.Lease = time.Minute
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 10 * time.Second
	}
	if opts.Client == nil {
		opts.Client = &http.Client{
			Timeout: opts.Timeout,
			// A receiver's redirect is not followed: the signature was made
			// for the configured URL.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		}
	}
	if opts.Logger == nil {
		opts.Logger = zap.NewNop()
	}
	return &Dispatcher{repo: repo, opts: opts, now: time.Now}
}

// MaxAttempts is the attempt cap per delivery.
func (d *Dispatcher) MaxAttempts() int { return len(d.opts.Backoff) + 1 }

// Emit records eventType for every enabled endpoint of orgID subscribed to it
// and starts delivering. Errors are logged, never returned: an event source
// (a run finishing, an approval being decided) must not fail on a webhook.
func (d *Dispatcher) Emit(ctx context.Context, orgID uuid.UUID, eventType string, data any) {
	if d == nil || orgID == uuid.Nil {
		return
	}
	// The source's context may be a request about to end; the write is short
	// and must not be cancelled with it.
	wctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	eps, err := d.repo.ListSubscribed(wctx, orgID, eventType)
	if err != nil {
		d.opts.Logger.Warn("webhook: list endpoints failed", zap.String("event", eventType), zap.Error(err))
		return
	}
	if len(eps) == 0 {
		return
	}
	env := Envelope{ID: uuid.New(), Type: eventType, CreatedAt: d.now().UTC(), OrgID: orgID, Data: data}
	for i := range eps {
		if _, err := d.enqueue(wctx, &eps[i], env); err != nil {
			d.opts.Logger.Warn("webhook: enqueue failed", zap.String("event", eventType),
				zap.String("endpoint_id", eps[i].ID.String()), zap.Error(err))
		}
	}
}

// Ping enqueues and delivers a "ping" event to one endpoint (the test action),
// regardless of its subscriptions, and returns the delivery after the attempt.
func (d *Dispatcher) Ping(ctx context.Context, ep *model.WebhookEndpoint) (*model.WebhookDelivery, error) {
	env := Envelope{ID: uuid.New(), Type: model.WebhookEventPing, CreatedAt: d.now().UTC(), OrgID: ep.OrgID,
		Data: map[string]any{"endpoint_id": ep.ID, "message": "JobShout webhook test"}}
	del, err := d.prepare(ctx, ep, env)
	if err != nil {
		return nil, err
	}
	d.attempt(ctx, del, ep)
	return del, nil
}

func (d *Dispatcher) prepare(ctx context.Context, ep *model.WebhookEndpoint, env Envelope) (*model.WebhookDelivery, error) {
	body, err := json.Marshal(env)
	if err != nil {
		return nil, fmt.Errorf("webhook: encode: %w", err)
	}
	lease := d.now().Add(d.opts.Lease)
	del := &model.WebhookDelivery{
		EndpointID: ep.ID, OrgID: ep.OrgID, EventID: env.ID, EventType: env.Type,
		Payload: body, Status: model.WebhookDeliveryPending, NextAttemptAt: &lease,
	}
	if err := d.repo.CreateDelivery(ctx, del); err != nil {
		return nil, err
	}
	return del, nil
}

func (d *Dispatcher) enqueue(ctx context.Context, ep *model.WebhookEndpoint, env Envelope) (*model.WebhookDelivery, error) {
	del, err := d.prepare(ctx, ep, env)
	if err != nil {
		return nil, err
	}
	epCopy := *ep
	if d.opts.Sync {
		d.attempt(ctx, del, &epCopy)
	} else {
		go d.attempt(context.Background(), del, &epCopy)
	}
	return del, nil
}

// Run polls for due retries every interval until ctx ends.
func (d *Dispatcher) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			d.RetryDue(ctx)
		}
	}
}

// RetryDue claims and attempts every due delivery once.
func (d *Dispatcher) RetryDue(ctx context.Context) int {
	due, err := d.repo.ClaimDue(ctx, 50, d.opts.Lease)
	if err != nil {
		d.opts.Logger.Warn("webhook: claim due deliveries failed", zap.Error(err))
		return 0
	}
	for i := range due {
		del := due[i]
		ep, err := d.repo.GetEndpointForDelivery(ctx, del.EndpointID)
		if err != nil {
			msg := "endpoint deleted"
			_ = d.repo.RecordAttempt(ctx, del.ID, model.WebhookDeliveryFailed, del.Attempts, nil, &msg, nil)
			continue
		}
		d.attempt(ctx, &del, ep)
	}
	return len(due)
}

// attempt POSTs one delivery and records the outcome, scheduling a retry with
// backoff on failure until the attempt cap.
func (d *Dispatcher) attempt(ctx context.Context, del *model.WebhookDelivery, ep *model.WebhookEndpoint) {
	del.Attempts++
	if !ep.Enabled {
		msg := "endpoint disabled"
		del.Status, del.LastError, del.NextAttemptAt = model.WebhookDeliveryFailed, &msg, nil
		_ = d.repo.RecordAttempt(ctx, del.ID, del.Status, del.Attempts, nil, del.LastError, nil)
		return
	}
	code, err := d.post(ctx, ep, del)
	if err == nil && code >= 200 && code < 300 {
		del.Status, del.LastStatusCode, del.LastError, del.NextAttemptAt = model.WebhookDeliverySucceeded, &code, nil, nil
		if rerr := d.repo.RecordAttempt(ctx, del.ID, del.Status, del.Attempts, &code, nil, nil); rerr != nil {
			d.opts.Logger.Warn("webhook: record success failed", zap.Error(rerr))
		}
		return
	}
	msg := ""
	if err != nil {
		msg = err.Error()
	} else {
		msg = "HTTP " + strconv.Itoa(code)
	}
	if len(msg) > 500 {
		msg = msg[:500]
	}
	var codePtr *int
	if code != 0 {
		codePtr = &code
	}
	del.LastStatusCode, del.LastError = codePtr, &msg
	if del.Attempts >= d.MaxAttempts() {
		del.Status, del.NextAttemptAt = model.WebhookDeliveryFailed, nil
	} else {
		next := d.now().Add(d.opts.Backoff[del.Attempts-1])
		del.Status, del.NextAttemptAt = model.WebhookDeliveryPending, &next
	}
	if rerr := d.repo.RecordAttempt(ctx, del.ID, del.Status, del.Attempts, codePtr, &msg, del.NextAttemptAt); rerr != nil {
		d.opts.Logger.Warn("webhook: record failure failed", zap.Error(rerr))
	}
	d.opts.Logger.Info("webhook: delivery attempt failed",
		zap.String("delivery_id", del.ID.String()), zap.String("event", del.EventType),
		zap.Int("attempt", del.Attempts), zap.String("status", del.Status), zap.String("error", msg))
}

func (d *Dispatcher) post(ctx context.Context, ep *model.WebhookEndpoint, del *model.WebhookDelivery) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, d.opts.Timeout)
	defer cancel()
	ts := d.now().Unix()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ep.URL, bytes.NewReader(del.Payload))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "JobShout-Webhooks/1")
	req.Header.Set(HeaderEvent, del.EventType)
	req.Header.Set(HeaderEventID, del.EventID.String())
	req.Header.Set(HeaderDelivery, del.ID.String())
	req.Header.Set(HeaderTimestamp, strconv.FormatInt(ts, 10))
	req.Header.Set(HeaderSignature, Sign(ep.Secret, ts, del.Payload))
	resp, err := d.opts.Client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	return resp.StatusCode, nil
}

// Sign returns the X-JobShout-Signature value for a body sent at ts.
func Sign(secret string, ts int64, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(strconv.FormatInt(ts, 10)))
	mac.Write([]byte("."))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// Verify checks a signature header in constant time (for receivers and tests).
func Verify(secret string, ts int64, body []byte, header string) bool {
	return hmac.Equal([]byte(Sign(secret, ts, body)), []byte(header))
}
