package harness

import (
	"fmt"
	"sort"
	"strings"
	"testing"
)

// Check is one assertion in an evaluation suite.
//
// Fatal marks a check whose failure is a defect rather than a regression in
// quality — "the mail agent sent a message without approval" is Fatal; "the
// draft was a bit terse" is not. A suite fails the build only on Fatal
// failures, so a non-Fatal check can record a known gap without blocking a
// merge on it.
type Check struct {
	Name  string
	Fatal bool
	// Fn returns nil when the check passes, or an error describing what was
	// expected and what happened. The message is written into the report, so
	// it should read as a sentence a person can act on.
	Fn func() error
}

// Outcome is the result of running one Check.
type Outcome struct {
	Name   string `json:"name"`
	Fatal  bool   `json:"fatal"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail,omitempty"`
}

// Report is the result of one suite, written to eval/out/<suite>.{json,md}.
type Report struct {
	Suite    string    `json:"suite"`
	Case     string    `json:"case,omitempty"`
	Outcomes []Outcome `json:"outcomes"`
	Notes    []string  `json:"notes,omitempty"`
	// Scores holds Tier 2 rubric marks (1–5) keyed by dimension. Always empty
	// for Tier 1: a hermetic suite has no opinion about prose quality.
	Scores map[string]int `json:"scores,omitempty"`
}

// Run executes every check and records the outcomes.
//
// It reports failures through t so a suite reads like an ordinary Go test,
// while also returning the Report so several cases can be aggregated into one
// written artefact.
func Run(t *testing.T, suite string, checks []Check) *Report {
	t.Helper()
	rep := &Report{Suite: suite}
	for _, c := range checks {
		err := c.Fn()
		out := Outcome{Name: c.Name, Fatal: c.Fatal, Passed: err == nil}
		if err != nil {
			out.Detail = err.Error()
			if c.Fatal {
				t.Errorf("[%s] FATAL %s: %v", suite, c.Name, err)
			} else {
				t.Logf("[%s] warn %s: %v", suite, c.Name, err)
			}
		}
		rep.Outcomes = append(rep.Outcomes, out)
	}
	return rep
}

// Note records free-form context on the report.
func (r *Report) Note(format string, args ...any) {
	r.Notes = append(r.Notes, fmt.Sprintf(format, args...))
}

// Failed counts failing checks, and failing Fatal checks.
func (r *Report) Failed() (total, fatal int) {
	for _, o := range r.Outcomes {
		if !o.Passed {
			total++
			if o.Fatal {
				fatal++
			}
		}
	}
	return
}

// Passed reports whether every Fatal check passed.
func (r *Report) Passed() bool {
	_, fatal := r.Failed()
	return fatal == 0
}

// --- assertion helpers ---------------------------------------------------
//
// These exist so checks read as one line each. Each returns an error phrased
// from the caller's point of view, because that string lands in the report.

// RequireContains fails unless every want appears in got (case-insensitive).
func RequireContains(what, got string, want ...string) error {
	low := strings.ToLower(got)
	var missing []string
	for _, w := range want {
		if w == "" {
			continue
		}
		if !strings.Contains(low, strings.ToLower(w)) {
			missing = append(missing, w)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("%s is missing %s; got: %s", what, quoteList(missing), truncate(got, 300))
	}
	return nil
}

// RequireAbsent fails if any unwanted string appears in got.
func RequireAbsent(what, got string, unwanted ...string) error {
	low := strings.ToLower(got)
	var found []string
	for _, u := range unwanted {
		if u == "" {
			continue
		}
		if strings.Contains(low, strings.ToLower(u)) {
			found = append(found, u)
		}
	}
	if len(found) > 0 {
		return fmt.Errorf("%s should not contain %s; got: %s", what, quoteList(found), truncate(got, 300))
	}
	return nil
}

// RequireZero fails unless n is zero — for "this must never have happened".
func RequireZero(what string, n int) error {
	if n != 0 {
		return fmt.Errorf("%s happened %d time(s); expected none", what, n)
	}
	return nil
}

// RequireEqual fails unless got == want.
func RequireEqual[T comparable](what string, got, want T) error {
	if got != want {
		return fmt.Errorf("%s = %v; want %v", what, got, want)
	}
	return nil
}

// RequireAtLeast fails unless got >= want.
func RequireAtLeast(what string, got, want int) error {
	if got < want {
		return fmt.Errorf("%s = %d; want at least %d", what, got, want)
	}
	return nil
}

// RequireSubset fails unless every element of got appears in allowed.
//
// This is the shape of the fabrication guard: the URLs in a draft must all come
// from the research brief, so an empty got passes and an invented one does not.
func RequireSubset(what string, got, allowed []string) error {
	set := make(map[string]bool, len(allowed))
	for _, a := range allowed {
		set[normaliseURL(a)] = true
	}
	var extra []string
	for _, g := range got {
		if !set[normaliseURL(g)] {
			extra = append(extra, g)
		}
	}
	if len(extra) > 0 {
		sort.Strings(extra)
		return fmt.Errorf("%s contains %s which came from no source", what, quoteList(extra))
	}
	return nil
}

func normaliseURL(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	s = strings.TrimSuffix(s, "/")
	s = strings.TrimSuffix(s, ".")
	return s
}

func quoteList(v []string) string {
	q := make([]string, len(v))
	for i, s := range v {
		q[i] = fmt.Sprintf("%q", s)
	}
	return strings.Join(q, ", ")
}
