package research

import "testing"

func topics(specs ...bool) []Topic {
	out := make([]Topic, 0, len(specs))
	for i, inFocus := range specs {
		out = append(out, Topic{
			Topic:   string(rune('A' + i)),
			InFocus: inFocus,
		})
	}
	return out
}

func names(ts []Topic) string {
	s := ""
	for _, t := range ts {
		s += t.Topic
	}
	return s
}

// On-subject topics come first, whatever order the model listed them in.
func TestSelectByFocusPrefersInFocusTopics(t *testing.T) {
	// B and D are in focus; A and C are not.
	got := selectByFocus(topics(false, true, false, true), 2, true)

	if names(got) != "BD" {
		t.Errorf("chose %q, want the two in-focus topics %q", names(got), "BD")
	}
}

// The decision the user made: when nothing is on-subject, write about the
// closest thing rather than producing nothing.
func TestSelectByFocusFallsBackToTheClosest(t *testing.T) {
	got := selectByFocus(topics(false, false, false), 2, true)

	if len(got) != 2 {
		t.Fatalf("got %d topics, want 2", len(got))
	}
	for _, tp := range got {
		if tp.InFocus {
			t.Errorf("topic %q was marked in-focus but none should be", tp.Topic)
		}
	}
}

// A short list of on-subject topics is topped up rather than truncated.
func TestSelectByFocusTopsUpFromOutsideTheAreas(t *testing.T) {
	got := selectByFocus(topics(true, false, false), 3, true)

	if len(got) != 3 {
		t.Fatalf("got %d topics, want 3", len(got))
	}
	if !got[0].InFocus {
		t.Errorf("the in-focus topic should lead, got %q", names(got))
	}
}

// Without focus areas the model's own ordering is authoritative — the flag
// means nothing, and reordering on it would be inventing a preference.
func TestSelectByFocusLeavesUnfocusedRunsAlone(t *testing.T) {
	got := selectByFocus(topics(false, true), 2, false)

	if names(got) != "AB" {
		t.Errorf("chose %q, want the original order %q", names(got), "AB")
	}
}

func TestSelectByFocusRespectsTheCount(t *testing.T) {
	if got := selectByFocus(topics(true, true, true), 1, true); len(got) != 1 {
		t.Errorf("got %d topics, want 1", len(got))
	}
	if got := selectByFocus(topics(true), 5, true); len(got) != 1 {
		t.Errorf("got %d topics, want the 1 available", len(got))
	}
	if got := selectByFocus(nil, 3, true); got != nil {
		t.Errorf("got %v, want nil for no topics", got)
	}
	if got := selectByFocus(topics(true), 0, true); got != nil {
		t.Errorf("got %v, want nil for a zero count", got)
	}
}
