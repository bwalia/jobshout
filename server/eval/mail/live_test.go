//go:build evallive

// Tier 2: the live Mail Agent suite.
//
// This talks to the real Gmail mailbox and the real model, and answers the one
// question the hermetic suite structurally cannot: is the draft any good? It is
// never part of `go test ./...` — run it deliberately:
//
//	go test -tags=evallive ./eval/mail/ -v
//
// It skips rather than fails when the mailbox is not configured, so a machine
// without Gmail credentials is not a broken build.
package mail_eval

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/jobshout/server/eval/harness"
	"github.com/jobshout/server/internal/mail"
)

func TestMailAgentLive(t *testing.T) {
	cfg := mail.LoadConfig()
	if !cfg.Configured() {
		t.Skip("mail: MAIL_* environment not configured; skipping the live suite")
	}
	orgID := os.Getenv("EVAL_ORG_ID")
	if orgID == "" {
		t.Skip("set EVAL_ORG_ID to the organisation whose mailbox should be evaluated")
	}
	org, err := uuid.Parse(orgID)
	if err != nil {
		t.Fatalf("EVAL_ORG_ID is not a UUID: %v", err)
	}

	suite := harness.NewSuite(t, "mail-live", ".")
	suite.Tier = "2"

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// The live suite is intentionally read-only about sending: it syncs, reads
	// the drafts that resulted, and reports. Approving a draft would send real
	// mail to a real person, which is not something a test may decide to do.
	svc, cleanup, err := liveMailService(ctx, org)
	if err != nil {
		t.Skipf("live mail service unavailable: %v", err)
	}
	defer cleanup()

	if err := svc.SyncNow(ctx, org); err != nil {
		t.Fatalf("sync: %v", err)
	}

	drafts, err := svc.ListPendingDrafts(ctx, org, defaultPage())
	if err != nil {
		t.Fatalf("list drafts: %v", err)
	}

	rep := suite.Case(t, "live_inbox_sync", []harness.Check{
		{Name: "sync_completed", Fatal: true, Fn: func() error { return nil }},
		{Name: "drafts_have_a_recipient", Fatal: true, Fn: func() error {
			for _, d := range drafts.Data {
				if d.ToEmail == "" {
					return harness.RequireContains("draft recipient", "", "an address")
				}
			}
			return nil
		}},
		{Name: "no_draft_claims_it_was_sent", Fatal: true, Fn: func() error {
			for _, d := range drafts.Data {
				if err := harness.RequireAbsent("draft body", d.Body,
					"i have sent", "message was sent", "i've emailed"); err != nil {
					return err
				}
			}
			return nil
		}},
	})
	rep.Note("Synced the live mailbox and inspected %d pending draft(s).", len(drafts.Data))

	// Drafts are written out for a human to read: "would you send this?" is the
	// judgement this tier exists to support, and it is not automatable.
	for i, d := range drafts.Data {
		rep.Note("Draft %d — to %s, subject %q:\n\n%s", i+1, d.ToEmail, d.Subject, d.Body)
	}
}
