package waflab

import (
	"testing"
)

func TestScoreMatrixBasic(t *testing.T) {
	results := []HostResult{
		{AttackID: "xss-script", AttackName: "XSS", Category: "xss", HostRole: "secure", Blocked: true, ExpectBlock: true},
		{AttackID: "xss-script", AttackName: "XSS", Category: "xss", HostRole: "open", Blocked: false, ExpectBlock: true},
		{AttackID: "sqli-union", AttackName: "SQLi", Category: "sqli", HostRole: "secure", Blocked: true, ExpectBlock: true},
		{AttackID: "sqli-union", AttackName: "SQLi", Category: "sqli", HostRole: "open", Blocked: false, ExpectBlock: true},
		{AttackID: "benign-home", AttackName: "Home", Category: "benign", HostRole: "secure", Blocked: false, ExpectBlock: false},
		{AttackID: "benign-home", AttackName: "Home", Category: "benign", HostRole: "open", Blocked: false, ExpectBlock: false},
		{AttackID: "bola-idor-business-logic", AttackName: "BOLA", Category: "bola", HostRole: "secure", Blocked: false, ExpectBlock: false},
		{AttackID: "bola-idor-business-logic", AttackName: "BOLA", Category: "bola", HostRole: "open", Blocked: false, ExpectBlock: false},
	}
	s := ScoreMatrix(results)
	if s.SecureBlocked != 2 || s.SecureExpected != 2 {
		t.Fatalf("secure blocked=%d expected=%d", s.SecureBlocked, s.SecureExpected)
	}
	if s.OpenLeaked != 2 {
		t.Fatalf("open leaked=%d", s.OpenLeaked)
	}
	if s.FalsePositives != 0 {
		t.Fatalf("fp=%d", s.FalsePositives)
	}
	if s.NotBlockedExpected != 1 {
		t.Fatalf("bola not_blocked_expected=%d", s.NotBlockedExpected)
	}
	if VerdictFor(results[6]) != "not_blocked_expected" {
		t.Fatalf("bola verdict=%s", VerdictFor(results[6]))
	}
}

func TestScoreFalsePositive(t *testing.T) {
	results := []HostResult{
		{AttackID: "xss-script", Category: "xss", HostRole: "secure", Blocked: true, ExpectBlock: true},
		{AttackID: "xss-script", Category: "xss", HostRole: "open", Blocked: false, ExpectBlock: true},
		{AttackID: "benign-home", Category: "benign", HostRole: "secure", Blocked: true, ExpectBlock: false},
		{AttackID: "benign-home", Category: "benign", HostRole: "open", Blocked: false, ExpectBlock: false},
	}
	s := ScoreMatrix(results)
	if s.FalsePositives != 1 {
		t.Fatalf("fp=%d want 1", s.FalsePositives)
	}
	if VerdictFor(results[2]) != "false_positive" {
		t.Fatalf("verdict=%s", VerdictFor(results[2]))
	}
}

func TestScoreSuspiciousPerfectBlock(t *testing.T) {
	var results []HostResult
	for i := 0; i < 5; i++ {
		id := string(rune('a' + i))
		results = append(results,
			HostResult{AttackID: id, Category: "xss", HostRole: "secure", Blocked: true, ExpectBlock: true},
			HostResult{AttackID: id, Category: "xss", HostRole: "open", Blocked: false, ExpectBlock: true},
		)
	}
	s := ScoreMatrix(results)
	if !s.Suspicious {
		t.Fatal("expected suspicious flag for 100% block + 0 FP")
	}
}
