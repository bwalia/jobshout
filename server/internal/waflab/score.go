package waflab

import (
	"github.com/jobshout/server/internal/model"
)

// HostResult is one attack outcome on one host.
type HostResult struct {
	AttackID     string
	AttackName   string
	Category     string
	HostRole     string // secure | open
	Host         string
	Method       string
	Path         string
	Payload      string
	Blocked      bool
	ExpectBlock  bool
	StatusCode   int
	WAFRule      string
	WAFViolation string
	SupportID    string
	LatencyMS    int
	Notes        string
}

// ScoreMatrix builds a WAFLabScore from per-host results.
//
// secureBlocked  — ExpectBlock attacks that were blocked on secure
// openLeaked     — ExpectBlock attacks that were NOT blocked on open (expected)
// falsePositives — ExpectBlock=false (benign) blocked on secure
// BOLA           — counted as not_blocked_expected when not blocked on secure
func ScoreMatrix(results []HostResult) model.WAFLabScore {
	byCat := map[string]*model.WAFLabCategoryScore{}
	ensure := func(cat string) *model.WAFLabCategoryScore {
		if byCat[cat] == nil {
			byCat[cat] = &model.WAFLabCategoryScore{}
		}
		return byCat[cat]
	}

	type pair struct {
		secure *HostResult
		open   *HostResult
		meta   HostResult
	}
	pairs := map[string]*pair{}
	for i := range results {
		r := results[i]
		p := pairs[r.AttackID]
		if p == nil {
			p = &pair{meta: r}
			pairs[r.AttackID] = p
		}
		switch r.HostRole {
		case "secure":
			cp := r
			p.secure = &cp
		case "open":
			cp := r
			p.open = &cp
		}
		p.meta = r
	}

	var score model.WAFLabScore
	score.ByCategory = map[string]model.WAFLabCategoryScore{}
	score.BOLANote = "BOLA/IDOR (GET /api/accounts/9999) is not blocked by design — object-level authorization has no signature in the request."

	attackIDs := orderedAttackIDs(results)
	for _, id := range attackIDs {
		p := pairs[id]
		if p == nil {
			continue
		}
		meta := p.meta
		if p.secure != nil {
			meta = *p.secure
		}
		score.AttacksTotal++
		cat := ensure(meta.Category)
		cat.Total++

		isBOLA := meta.Category == "bola" || id == "bola-idor-business-logic"
		isBenign := meta.Category == "benign" || (!meta.ExpectBlock && !isBOLA)

		if meta.ExpectBlock {
			score.SecureExpected++
			score.OpenExpectedLeak++
			if p.secure != nil && p.secure.Blocked {
				score.SecureBlocked++
				cat.SecureBlocked++
			}
			if p.open != nil && !p.open.Blocked {
				score.OpenLeaked++
				cat.OpenLeaked++
			}
			continue
		}

		// ExpectBlock=false
		if isBOLA {
			if p.secure == nil || !p.secure.Blocked {
				score.NotBlockedExpected++
			} else {
				// Unexpected block of BOLA — treat as false positive signal
				score.FalsePositives++
				cat.FalsePositives++
			}
			continue
		}

		if isBenign && p.secure != nil && p.secure.Blocked {
			score.FalsePositives++
			cat.FalsePositives++
		}
	}

	for k, v := range byCat {
		score.ByCategory[k] = *v
	}

	// 100% block rate with zero FPs on a non-empty attack set looks wrong.
	if score.SecureExpected > 0 &&
		score.SecureBlocked == score.SecureExpected &&
		score.FalsePositives == 0 &&
		score.AttacksTotal > 3 {
		score.Suspicious = true
		score.SuspiciousReason = "100% of expected attacks blocked with zero false positives — matrix looks suspicious; inspect raw responses"
	}
	return score
}

func orderedAttackIDs(results []HostResult) []string {
	seen := map[string]bool{}
	var ids []string
	for _, r := range results {
		if seen[r.AttackID] {
			continue
		}
		seen[r.AttackID] = true
		ids = append(ids, r.AttackID)
	}
	return ids
}

// VerdictFor returns a stable verdict string for one host result.
func VerdictFor(r HostResult) string {
	isBOLA := r.Category == "bola" || r.AttackID == "bola-idor-business-logic"
	if isBOLA {
		if r.Blocked {
			return "false_positive"
		}
		return "not_blocked_expected"
	}
	if !r.ExpectBlock {
		if r.Blocked {
			return "false_positive"
		}
		return "allowed"
	}
	if r.HostRole == "open" {
		if r.Blocked {
			return "unexpected_block"
		}
		return "leaked"
	}
	if r.Blocked {
		return "blocked"
	}
	return "leaked"
}
