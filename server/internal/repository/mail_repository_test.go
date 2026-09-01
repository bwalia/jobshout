package repository

import (
	"strings"
	"testing"
)

func TestClaimDueConnectionsSQLIncludesErrorStatus(t *testing.T) {
	if !strings.Contains(claimDuePredicate, "status IN ('connected', 'error')") {
		t.Fatalf("must claim error mailboxes, not only connected: %s", claimDuePredicate)
	}
	if !strings.Contains(claimDuePredicate, "refresh_token_enc IS NOT NULL") {
		t.Fatal("must require a refresh token")
	}
}

func TestUpsertConnectionSQLWritesSyncLease(t *testing.T) {
	if !strings.Contains(upsertConnectionSQL, "sync_lease_until") {
		t.Fatal("failed sync and EnqueueSync must persist sync_lease_until, including NULL")
	}
	if !strings.Contains(upsertConnectionSQL, "sync_lease_until = EXCLUDED.sync_lease_until") {
		t.Fatal("ON CONFLICT must update sync_lease_until so a nilled lease actually clears")
	}
}

func TestMailConnSQLIncludesKnowledgePlaybook(t *testing.T) {
	for _, col := range []string{"watch_knowledge_urls", "research_focus", "reply_instructions"} {
		if !strings.Contains(mailConnColumns, col) {
			t.Errorf("mailConnColumns missing %s", col)
		}
		if !strings.Contains(upsertConnectionSQL, col) {
			t.Errorf("upsertConnectionSQL missing %s", col)
		}
		if !strings.Contains(updateConnectionMetaSQL, col) {
			t.Errorf("updateConnectionMetaSQL missing %s", col)
		}
	}
}
