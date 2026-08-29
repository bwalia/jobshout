package mail

import (
	"context"
	"testing"
)

func TestSimulatedGmail_PushAndFilterFrom(t *testing.T) {
	g := NewSimulatedGmail()
	g.Push(
		InboxMessage{GmailThreadID: "a", FromEmail: "client@acme.com", Subject: "price"},
		InboxMessage{GmailThreadID: "b", FromEmail: "news@list.com", Subject: "sale"},
	)
	all, err := g.ListMessages(context.Background(), "tok", "is:unread", 25)
	if err != nil || len(all) != 2 {
		t.Fatalf("all = %d err=%v", len(all), err)
	}
	filtered, err := g.ListMessages(context.Background(), "tok", "is:unread (from:client@acme.com)", 25)
	if err != nil || len(filtered) != 1 || filtered[0].FromEmail != "client@acme.com" {
		t.Fatalf("filtered = %+v err=%v", filtered, err)
	}
}

func TestSimulatedGmail_SendRecords(t *testing.T) {
	g := NewSimulatedGmail()
	id, err := g.Send(context.Background(), "tok", OutboundMessage{To: "a@b.c", Subject: "Re: hi"})
	if err != nil || id == "" {
		t.Fatal(err)
	}
	if len(g.Sent()) != 1 {
		t.Fatalf("sent %d", len(g.Sent()))
	}
}
