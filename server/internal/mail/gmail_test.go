package mail

import "testing"

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
