package harness

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// OutDir is where suite artefacts are written, relative to the eval tree.
const OutDir = "out"

// Suite aggregates the reports of several cases and writes one artefact.
//
// A suite is created with NewSuite, accumulates per-case Reports, and writes
// itself via t.Cleanup, so a test file never has to remember to flush.
type Suite struct {
	Name    string    `json:"suite"`
	Tier    string    `json:"tier"`
	Reports []*Report `json:"reports"`
	dir     string
}

// NewSuite starts a suite that writes to <dir>/out/<name>.{json,md} when the
// test finishes. dir is usually "." — the package directory of the suite.
func NewSuite(t *testing.T, name, dir string) *Suite {
	t.Helper()
	s := &Suite{Name: name, Tier: "1", dir: dir}
	t.Cleanup(func() {
		if err := s.Write(); err != nil {
			t.Logf("harness: writing %s report: %v", name, err)
		}
	})
	return s
}

// Add records a case report.
func (s *Suite) Add(r *Report) *Report {
	s.Reports = append(s.Reports, r)
	return r
}

// Case runs a set of checks under a case name and records the result.
func (s *Suite) Case(t *testing.T, name string, checks []Check) *Report {
	t.Helper()
	r := Run(t, s.Name, checks)
	r.Case = name
	return s.Add(r)
}

// Totals counts passed, failed and fatally-failed checks across the suite.
func (s *Suite) Totals() (passed, failed, fatal int) {
	for _, r := range s.Reports {
		for _, o := range r.Outcomes {
			if o.Passed {
				passed++
				continue
			}
			failed++
			if o.Fatal {
				fatal++
			}
		}
	}
	return
}

// Write emits the JSON and Markdown artefacts.
//
// Failure to write is never fatal to a test run: the artefact is a convenience
// for a human reading results, not the result itself. The assertions already
// ran through *testing.T.
func (s *Suite) Write() error {
	dir := filepath.Join(s.dir, OutDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	base := filepath.Join(dir, sanitiseName(s.Name))

	blob, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(base+".json", blob, 0o644); err != nil {
		return err
	}
	return os.WriteFile(base+".md", []byte(s.Markdown()), 0o644)
}

// Markdown renders the suite as a report a person can read in a PR.
func (s *Suite) Markdown() string {
	passed, failed, fatal := s.Totals()
	var b strings.Builder

	fmt.Fprintf(&b, "# Eval report — %s (tier %s)\n\n", s.Name, s.Tier)
	verdict := "PASS"
	if fatal > 0 {
		verdict = "FAIL"
	}
	fmt.Fprintf(&b, "**%s** — %d passed, %d failed (%d fatal)\n\n", verdict, passed, failed, fatal)

	for _, r := range s.Reports {
		name := r.Case
		if name == "" {
			name = "(unnamed case)"
		}
		fmt.Fprintf(&b, "## %s\n\n", name)
		fmt.Fprintf(&b, "| check | fatal | result | detail |\n|---|---|---|---|\n")
		for _, o := range r.Outcomes {
			mark := "✅"
			if !o.Passed {
				mark = "❌"
			}
			fatalMark := ""
			if o.Fatal {
				fatalMark = "yes"
			}
			fmt.Fprintf(&b, "| %s | %s | %s | %s |\n",
				o.Name, fatalMark, mark, escapePipes(o.Detail))
		}
		b.WriteString("\n")

		if len(r.Scores) > 0 {
			keys := make([]string, 0, len(r.Scores))
			for k := range r.Scores {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			b.WriteString("| rubric | score (1–5) |\n|---|---|\n")
			for _, k := range keys {
				fmt.Fprintf(&b, "| %s | %d |\n", k, r.Scores[k])
			}
			b.WriteString("\n")
		}

		for _, n := range r.Notes {
			fmt.Fprintf(&b, "> %s\n\n", n)
		}
	}
	return b.String()
}

func escapePipes(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	return strings.ReplaceAll(s, "\n", " ")
}

func sanitiseName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "-")
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return '-'
		}
	}, s)
}
