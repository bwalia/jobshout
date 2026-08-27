package mail

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestRulesQueryDefaultUnreadInbox(t *testing.T) {
	q := RulesQuery(nil, nil, nil)
	if q != "is:unread in:inbox newer_than:7d" {
		t.Errorf("got %q", q)
	}
}

func TestRulesQueryIncludesSenders(t *testing.T) {
	q := RulesQuery(nil, []string{"alex@c.com"}, nil)
	if q != `is:unread in:inbox newer_than:7d (from:alex@c.com)` {
		t.Errorf("got %q", q)
	}
}

func TestGmailMessageInternalDateString(t *testing.T) {
	raw := []byte(`{"id":"1","threadId":"t","snippet":"hi","internalDate":"1710000000000","payload":{"mimeType":"text/plain","headers":[],"body":{"data":""}}}`)
	var m gmailMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	msg := parseGmailMessage(m)
	if msg.GmailThreadID != "t" {
		t.Fatalf("thread %q", msg.GmailThreadID)
	}
	want := time.UnixMilli(1710000000000)
	if !msg.ReceivedAt.Equal(want) {
		t.Fatalf("received %v want %v", msg.ReceivedAt, want)
	}
}

func TestGmailMessageInternalDateNumber(t *testing.T) {
	raw := []byte(`{"id":"1","threadId":"t","internalDate":1710000000000,"payload":{"headers":[]}}`)
	var m gmailMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if m.Internal.ms != 1710000000000 {
		t.Fatalf("ms %d", m.Internal.ms)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func jsonResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

func TestListMessagesSkipsUnreadableMessage(t *testing.T) {
	listBody := `{"messages":[{"id":"bad","threadId":"t1"},{"id":"good","threadId":"t2"}]}`
	goodMsg := `{"id":"good","threadId":"t2","snippet":"ok","internalDate":"1710000000000","payload":{"mimeType":"text/plain","headers":[{"name":"Subject","value":"Hi"}],"body":{"data":""}}}`
	badMsg := `{"id":"bad","threadId":"t1","internalDate":"not-a-number","payload":{"headers":[]}}`

	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if strings.Contains(req.URL.Path, "/messages/") {
			if strings.Contains(req.URL.Path, "/bad") {
				return jsonResponse(badMsg), nil
			}
			return jsonResponse(goodMsg), nil
		}
		return jsonResponse(listBody), nil
	})

	core, logs := observer.New(zapcore.WarnLevel)
	g := NewGmailAPI(&http.Client{Transport: rt}, zap.New(core))
	msgs, err := g.ListMessages(context.Background(), "token", "", 25)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 || msgs[0].GmailThreadID != "t2" {
		t.Fatalf("got %+v", msgs)
	}
	if logs.FilterMessage("mail: skipping gmail message").Len() != 1 {
		t.Fatalf("want one skip warning, got %d", logs.Len())
	}
}
