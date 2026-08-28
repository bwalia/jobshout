package chatagent

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
	"time"

	"github.com/jobshout/server/internal/model"
)

func newConfirmToken() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return hex.EncodeToString([]byte(time.Now().UTC().Format(time.RFC3339Nano)))
	}
	return hex.EncodeToString(b[:])
}

func isAffirmative(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "yes", "y", "ok", "okay", "approve", "confirm", "do it", "proceed", "go ahead", "sure":
		return true
	default:
		return false
	}
}

func isNegative(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "no", "n", "cancel", "stop", "don't", "dont", "never", "nope", "reject":
		return true
	default:
		return false
	}
}

func isAbandon(s string) bool {
	l := strings.ToLower(strings.TrimSpace(s))
	if isNegative(l) {
		return true
	}
	for _, p := range []string{"never mind", "nevermind", "forget it", "forget that"} {
		if strings.Contains(l, p) {
			return true
		}
	}
	return false
}

func looksLikeNewRequest(s string) bool {
	l := strings.ToLower(strings.TrimSpace(s))
	for _, p := range []string{"list ", "create ", "run ", "show ", "delete ", "start "} {
		if strings.HasPrefix(l, p) {
			return true
		}
	}
	return false
}

func mergePendingArgs(p *model.PendingAction, userText string) map[string]any {
	args := map[string]any{}
	for k, v := range p.Args {
		args[k] = v
	}
	text := strings.TrimSpace(userText)
	if text == "" || len(p.Missing) == 0 {
		return args
	}
	if len(p.Missing) == 1 {
		args[p.Missing[0]] = text
		return args
	}
	// Several slots: put the whole reply in the first still-empty one.
	for _, slot := range p.Missing {
		if asString(args[slot]) == "" {
			args[slot] = text
			break
		}
	}
	return args
}

func pendingStillMissing(p *model.PendingAction, args map[string]any) bool {
	if p == nil {
		return false
	}
	for _, slot := range p.Missing {
		if asString(args[slot]) == "" {
			return true
		}
	}
	return false
}

func copyArgs(in map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range in {
		out[k] = v
	}
	return out
}

func looksAffirmativeSuccess(s string) bool {
	l := strings.ToLower(s)
	for _, w := range []string{
		"created", "deleted", "started", "completed", "done", "published",
		"cancelled", "canceled", "assigned", "updated successfully", "all set",
	} {
		if strings.Contains(l, w) {
			return true
		}
	}
	return false
}
