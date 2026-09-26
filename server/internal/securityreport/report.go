// Package securityreport versions security test runs and builds PDF reports.
package securityreport

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/reportpdf"
)

const (
	KindPentest = "pentest"
	KindWAFLab  = "waf_lab"

	EventDetected  = "detected"
	EventStillOpen = "still_open"
	EventFixed     = "fixed"
)

// AppVersion returns the deployed JobShout version stamp (Helm image.tag / APP_VERSION).
func AppVersion() string {
	v := strings.TrimSpace(os.Getenv("APP_VERSION"))
	if v == "" || v == "latest" {
		return "unknown"
	}
	return v
}

// FindingRef is a fingerprintable finding for cross-run diff.
type FindingRef struct {
	Key      string
	Severity string
	Title    string
	Detail   string
}

// Diff produces detected / still_open / fixed events comparing previous → current.
func Diff(prev, curr map[string]FindingRef) []model.SecurityFindingEvent {
	out := make([]model.SecurityFindingEvent, 0, len(prev)+len(curr))
	for k, c := range curr {
		ev := EventDetected
		if _, ok := prev[k]; ok {
			ev = EventStillOpen
		}
		out = append(out, model.SecurityFindingEvent{
			FindingKey: k,
			Event:      ev,
			Severity:   c.Severity,
			Title:      c.Title,
			Detail:     c.Detail,
		})
	}
	for k, p := range prev {
		if _, ok := curr[k]; ok {
			continue
		}
		out = append(out, model.SecurityFindingEvent{
			FindingKey: k,
			Event:      EventFixed,
			Severity:   p.Severity,
			Title:      p.Title,
			Detail:     p.Detail,
		})
	}
	return out
}

// PentestKey fingerprints a finding within a target.
func PentestKey(f model.PentestFinding) string {
	if f.ExternalID != nil && strings.TrimSpace(*f.ExternalID) != "" {
		return "ext:" + strings.TrimSpace(*f.ExternalID)
	}
	title := strings.ToLower(strings.TrimSpace(f.Title))
	cat := strings.ToLower(strings.TrimSpace(f.Category))
	return "t:" + title + "|c:" + cat
}

// PentestRefs maps findings by key.
func PentestRefs(findings []model.PentestFinding) map[string]FindingRef {
	m := make(map[string]FindingRef, len(findings))
	for _, f := range findings {
		k := PentestKey(f)
		m[k] = FindingRef{
			Key: k, Severity: f.Severity, Title: f.Title,
			Detail: truncate(f.Description, 240),
		}
	}
	return m
}

// WAFLeakKey fingerprints a secure-host leak (vulnerability for the lab).
func WAFLeakKey(r model.WAFLabResult) string {
	return "atk:" + strings.TrimSpace(r.AttackID)
}

// WAFLeakRefs collects secure-host leaked attacks as findings.
func WAFLeakRefs(results []model.WAFLabResult) map[string]FindingRef {
	m := make(map[string]FindingRef)
	for _, r := range results {
		if r.HostRole != "secure" || r.Verdict != "leaked" {
			continue
		}
		k := WAFLeakKey(r)
		m[k] = FindingRef{
			Key: k, Severity: "high", Title: r.AttackName,
			Detail: fmt.Sprintf("%s %s %s → HTTP %d", r.Category, r.Method, r.Path, r.StatusCode),
		}
	}
	return m
}

// FormatTime stamps a time for reports.
func FormatTime(t *time.Time, fallback time.Time) string {
	if t != nil {
		return t.UTC().Format(time.RFC3339)
	}
	return fallback.UTC().Format(time.RFC3339)
}

// ReportLabel is e.g. "v3 @ v1.2.0 (2026-09-19T…Z)".
func ReportLabel(seq *int, appVersion *string, when time.Time) string {
	parts := []string{}
	if seq != nil && *seq > 0 {
		parts = append(parts, fmt.Sprintf("report v%d", *seq))
	}
	if appVersion != nil && strings.TrimSpace(*appVersion) != "" {
		parts = append(parts, "app "+strings.TrimSpace(*appVersion))
	}
	parts = append(parts, when.UTC().Format(time.RFC3339))
	return strings.Join(parts, " · ")
}

// BuildPentestPDF renders a versioned pentest PDF.
func BuildPentestPDF(run *model.PentestRun, findings []model.PentestFinding, events []model.SecurityFindingEvent) ([]byte, string, error) {
	when := run.CreatedAt
	if run.CompletedAt != nil {
		when = *run.CompletedAt
	}
	meta := []reportpdf.Meta{
		{Label: "Report version", Value: ReportLabel(run.ReportSeq, run.AppVersion, when)},
		{Label: "Run ID", Value: run.ID.String()},
		{Label: "Target", Value: run.Target},
		{Label: "Scan mode", Value: run.ScanMode},
		{Label: "Status", Value: run.Status},
		{Label: "Created", Value: FormatTime(nil, run.CreatedAt)},
		{Label: "Completed", Value: FormatTime(run.CompletedAt, run.CreatedAt)},
		{Label: "Findings", Value: fmt.Sprintf("%d (high %d / medium %d / low %d)",
			run.FindingCount, run.HighSeverity, run.MediumSeverity, run.LowSeverity)},
	}
	if run.TargetEngaged != nil {
		meta = append(meta, reportpdf.Meta{Label: "Target engaged", Value: fmt.Sprintf("%v", *run.TargetEngaged)})
	}

	sections := []reportpdf.Section{
		{Heading: "Executive summary", Lines: pentestSummary(run, findings, events)},
		{Heading: "Change log (vs previous report)", Lines: eventLines(events)},
		{Heading: "Findings", Lines: pentestFindingLines(findings)},
	}
	if run.ReportMarkdown != nil && strings.TrimSpace(*run.ReportMarkdown) != "" {
		sections = append(sections, reportpdf.Section{
			Heading: "Scanner narrative",
			Lines:   wrapNarrative(*run.ReportMarkdown, 40),
		})
	}

	pdf, err := reportpdf.Render(reportpdf.Doc{
		Title:    "Penetration Test Report",
		Subtitle: "JobShout Security Tester",
		Meta:     meta,
		Sections: sections,
	})
	if err != nil {
		return nil, "", err
	}
	name := reportpdf.Filename("pentest", run.Target, seqPart(run.ReportSeq), when.Format("20060102-150405"))
	return pdf, name, nil
}

// BuildWAFLabPDF renders a versioned WAF Efficacy Lab PDF.
func BuildWAFLabPDF(run *model.WAFLabRun, results []model.WAFLabResult, events []model.SecurityFindingEvent) ([]byte, string, error) {
	when := run.CreatedAt
	if run.CompletedAt != nil {
		when = *run.CompletedAt
	}
	meta := []reportpdf.Meta{
		{Label: "Report version", Value: ReportLabel(run.ReportSeq, run.AppVersion, when)},
		{Label: "Run ID", Value: run.ID.String()},
		{Label: "Secure host", Value: run.SecureHost},
		{Label: "Open host", Value: run.OpenHost},
		{Label: "Mode", Value: run.Mode},
		{Label: "Attack set", Value: run.AttackSet},
		{Label: "Status", Value: run.Status},
		{Label: "Created", Value: FormatTime(nil, run.CreatedAt)},
		{Label: "Completed", Value: FormatTime(run.CompletedAt, run.CreatedAt)},
	}
	if run.Score != nil {
		s := run.Score
		meta = append(meta, reportpdf.Meta{
			Label: "Score",
			Value: fmt.Sprintf("secure %d/%d blocked · open %d/%d leaked · FP %d",
				s.SecureBlocked, s.SecureExpected, s.OpenLeaked, s.OpenExpectedLeak, s.FalsePositives),
		})
	}

	sections := []reportpdf.Section{
		{Heading: "Executive summary", Lines: wafSummary(run, results, events)},
		{Heading: "Change log (leaks vs previous report)", Lines: eventLines(events)},
		{Heading: "Secure-host results", Lines: wafResultLines(results, "secure")},
		{Heading: "Open-host results", Lines: wafResultLines(results, "open")},
	}

	pdf, err := reportpdf.Render(reportpdf.Doc{
		Title:    "WAF Efficacy Lab Report",
		Subtitle: "JobShout WAF Efficacy Lab",
		Meta:     meta,
		Sections: sections,
	})
	if err != nil {
		return nil, "", err
	}
	name := reportpdf.Filename("waf-lab", run.SecureHost, seqPart(run.ReportSeq), when.Format("20060102-150405"))
	return pdf, name, nil
}

func seqPart(seq *int) string {
	if seq == nil || *seq <= 0 {
		return "r"
	}
	return fmt.Sprintf("v%d", *seq)
}

func pentestSummary(run *model.PentestRun, findings []model.PentestFinding, events []model.SecurityFindingEvent) []string {
	nNew, nOpen, nFixed := countEvents(events)
	lines := []string{
		fmt.Sprintf("Scan of %s finished with status %s.", run.Target, run.Status),
		fmt.Sprintf("%d confirmed findings in this report.", len(findings)),
	}
	if len(events) > 0 {
		lines = append(lines, fmt.Sprintf("Since previous report: %d new, %d still open, %d fixed.", nNew, nOpen, nFixed))
	} else {
		lines = append(lines, "No prior completed report for this target — this is the baseline.")
	}
	return lines
}

func wafSummary(run *model.WAFLabRun, results []model.WAFLabResult, events []model.SecurityFindingEvent) []string {
	leaks := 0
	for _, r := range results {
		if r.HostRole == "secure" && r.Verdict == "leaked" {
			leaks++
		}
	}
	nNew, nOpen, nFixed := countEvents(events)
	lines := []string{
		fmt.Sprintf("Lab for secure host %s finished with status %s.", run.SecureHost, run.Status),
		fmt.Sprintf("%d secure-host leak(s) in this report.", leaks),
	}
	if len(events) > 0 {
		lines = append(lines, fmt.Sprintf("Since previous report: %d new leaks, %d still open, %d fixed.", nNew, nOpen, nFixed))
	} else {
		lines = append(lines, "No prior completed report for this host — this is the baseline.")
	}
	return lines
}

func countEvents(events []model.SecurityFindingEvent) (nNew, nOpen, nFixed int) {
	for _, e := range events {
		switch e.Event {
		case EventDetected:
			nNew++
		case EventStillOpen:
			nOpen++
		case EventFixed:
			nFixed++
		}
	}
	return
}

func eventLines(events []model.SecurityFindingEvent) []string {
	if len(events) == 0 {
		return []string{"(no prior report to compare)"}
	}
	order := []string{EventDetected, EventStillOpen, EventFixed}
	labels := map[string]string{
		EventDetected: "NEW", EventStillOpen: "STILL OPEN", EventFixed: "FIXED",
	}
	var lines []string
	for _, kind := range order {
		for _, e := range events {
			if e.Event != kind {
				continue
			}
			sev := e.Severity
			if sev == "" {
				sev = "-"
			}
			lines = append(lines, fmt.Sprintf("[%s] (%s) %s", labels[kind], sev, e.Title))
		}
	}
	return lines
}

func pentestFindingLines(findings []model.PentestFinding) []string {
	if len(findings) == 0 {
		return []string{"No confirmed findings."}
	}
	var lines []string
	for i, f := range findings {
		lines = append(lines, fmt.Sprintf("%d. [%s] %s (%s)", i+1, strings.ToUpper(f.Severity), f.Title, f.Category))
		if d := strings.TrimSpace(f.Description); d != "" {
			lines = append(lines, "   "+truncate(d, 300))
		}
	}
	return lines
}

func wafResultLines(results []model.WAFLabResult, role string) []string {
	var lines []string
	for _, r := range results {
		if r.HostRole != role {
			continue
		}
		lines = append(lines, fmt.Sprintf("[%s] %s — %s %s → HTTP %d (%s)",
			r.Verdict, r.AttackName, r.Method, r.Path, r.StatusCode, r.Category))
	}
	if len(lines) == 0 {
		return []string{"(none)"}
	}
	return lines
}

func wrapNarrative(md string, maxLines int) []string {
	md = strings.ReplaceAll(md, "\r\n", "\n")
	raw := strings.Split(md, "\n")
	var lines []string
	for _, l := range raw {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		// strip light markdown
		l = strings.TrimLeft(l, "#*- ")
		lines = append(lines, truncate(l, 110))
		if len(lines) >= maxLines {
			lines = append(lines, "… (truncated)")
			break
		}
	}
	if len(lines) == 0 {
		return []string{"(empty)"}
	}
	return lines
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

// StampEvents fills org/run metadata on diff events.
func StampEvents(events []model.SecurityFindingEvent, orgID uuid.UUID, kind, subject string, runID uuid.UUID, seq *int, appVer *string) []model.SecurityFindingEvent {
	out := make([]model.SecurityFindingEvent, len(events))
	for i, e := range events {
		e.ID = uuid.New()
		e.OrgID = orgID
		e.AgentKind = kind
		e.SubjectKey = subject
		e.RunID = runID
		e.ReportSeq = seq
		e.AppVersion = appVer
		out[i] = e
	}
	return out
}
