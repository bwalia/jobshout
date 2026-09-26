package career

import (
	"fmt"
	"strings"

	"github.com/jobshout/server/internal/model"
)

// AllowedTransitions is the canonical tracker state machine.
var AllowedTransitions = map[string][]string{
	model.CareerStatusEvaluated: {model.CareerStatusApplied, model.CareerStatusDiscarded, model.CareerStatusSkip, model.CareerStatusRejected},
	model.CareerStatusApplied:   {model.CareerStatusResponded, model.CareerStatusInterview, model.CareerStatusRejected, model.CareerStatusDiscarded, model.CareerStatusSkip},
	model.CareerStatusResponded: {model.CareerStatusInterview, model.CareerStatusRejected, model.CareerStatusDiscarded, model.CareerStatusOffer},
	model.CareerStatusInterview: {model.CareerStatusOffer, model.CareerStatusRejected, model.CareerStatusDiscarded, model.CareerStatusApplied},
	model.CareerStatusOffer:     {model.CareerStatusHired, model.CareerStatusRejected, model.CareerStatusDiscarded},
	model.CareerStatusRejected:  {model.CareerStatusEvaluated}, // reopen
	model.CareerStatusDiscarded: {model.CareerStatusEvaluated},
	model.CareerStatusSkip:      {model.CareerStatusEvaluated},
	model.CareerStatusHired:     {},
}

// CanTransition reports whether from → to is a legal tracker move.
func CanTransition(from, to string) bool {
	from, to = strings.TrimSpace(from), strings.TrimSpace(to)
	if from == to {
		return true
	}
	for _, next := range AllowedTransitions[from] {
		if next == to {
			return true
		}
	}
	return false
}

func ValidateTransition(from, to string) error {
	if !CanTransition(from, to) {
		return fmt.Errorf("career: cannot move from %s to %s", from, to)
	}
	return nil
}

func KnownStatus(s string) bool {
	switch s {
	case model.CareerStatusEvaluated, model.CareerStatusApplied, model.CareerStatusResponded,
		model.CareerStatusInterview, model.CareerStatusOffer, model.CareerStatusRejected,
		model.CareerStatusDiscarded, model.CareerStatusSkip, model.CareerStatusHired:
		return true
	}
	return false
}
