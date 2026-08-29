package mail

import (
	"context"
	"strings"
	"sync"
	"time"
)

// SimulatedGmail is an in-process Gmail stand-in for local API testing.
// It never talks to Google.
type SimulatedGmail struct {
	mu       sync.Mutex
	messages []InboxMessage
	sent     []OutboundMessage
	email    string
}

// NewSimulatedGmail returns an empty fake mailbox.
func NewSimulatedGmail() *SimulatedGmail {
	return &SimulatedGmail{email: "sim@jobshout.local"}
}

func (g *SimulatedGmail) Push(msgs ...InboxMessage) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.messages = append(g.messages, msgs...)
}

func (g *SimulatedGmail) ClearInbox() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.messages = nil
}

func (g *SimulatedGmail) Sent() []OutboundMessage {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := make([]OutboundMessage, len(g.sent))
	copy(out, g.sent)
	return out
}

func (g *SimulatedGmail) ExchangeCode(context.Context, string, string, string, string) (TokenSet, error) {
	return TokenSet{AccessToken: "sim-access", RefreshToken: "sim-refresh", Expiry: time.Now().Add(time.Hour)}, nil
}

func (g *SimulatedGmail) Refresh(context.Context, string, string, string) (TokenSet, error) {
	return TokenSet{AccessToken: "sim-access", RefreshToken: "sim-refresh", Expiry: time.Now().Add(time.Hour)}, nil
}

func (g *SimulatedGmail) Profile(context.Context, string) (string, error) {
	return g.email, nil
}

func (g *SimulatedGmail) ListMessages(_ context.Context, _ string, query string, limit int) ([]InboxMessage, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	var fromFilters []string
	for _, part := range strings.Fields(query) {
		part = strings.Trim(part, `()"'`)
		if after, ok := strings.CutPrefix(part, "from:"); ok {
			fromFilters = append(fromFilters, strings.Trim(after, `"'`))
		}
	}
	var out []InboxMessage
	for _, m := range g.messages {
		if len(fromFilters) > 0 {
			ok := false
			for _, f := range fromFilters {
				if strings.EqualFold(m.FromEmail, f) {
					ok = true
					break
				}
			}
			if !ok {
				continue
			}
		}
		out = append(out, m)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (g *SimulatedGmail) Send(_ context.Context, _ string, msg OutboundMessage) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.sent = append(g.sent, msg)
	return "sim-sent-" + time.Now().Format("150405"), nil
}
