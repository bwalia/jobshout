package career

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/jobshout/server/internal/research"
)

// Fetcher retrieves a page as readable text. Satisfied by research.Client.
type Fetcher interface {
	Fetch(ctx context.Context, rawURL string) (*research.Document, error)
}

// JobListing is extracted JD text plus the metadata we could parse without an LLM.
type JobListing struct {
	URL        string
	Company    string
	Title      string
	Text       string
	Via        string // greenhouse | ashby | lever | agency | web
	Agency     string
	Live       bool
	DeadReason string
}

var deadPhrases = []string{
	"no longer available",
	"this position has been filled",
	"this job is no longer",
	"job not found",
	"posting has expired",
	"this requisition is closed",
	"no longer accepting applications",
	"this opportunity has closed",
}

var greenhouseJob = regexp.MustCompile(`(?i)(?:job-)?boards\.greenhouse\.io/([^/]+)/jobs/(\d+)`)
var leverJob = regexp.MustCompile(`(?i)jobs\.lever\.co/([^/]+)/([0-9a-f-]+)`)
var ashbyJob = regexp.MustCompile(`(?i)jobs\.ashbyhq\.com/([^/]+)/([0-9a-f-]+)`)

// Extract builds a listing from pasted text and/or a URL.
func Extract(ctx context.Context, fetcher Fetcher, httpc *http.Client, jobURL, jdText string) (*JobListing, error) {
	jobURL = strings.TrimSpace(jobURL)
	jdText = strings.TrimSpace(jdText)
	if jobURL == "" && jdText == "" {
		return nil, fmt.Errorf("career: job URL or JD text is required")
	}

	listing := &JobListing{URL: jobURL, Text: jdText, Live: true, Via: "web"}
	if jobURL != "" {
		if parsed, err := url.Parse(jobURL); err == nil && parsed.Host != "" {
			listing.Company = guessCompanyFromHost(parsed.Host)
		}
		if ats := tryATSJSON(ctx, httpc, jobURL); ats != nil {
			mergeListing(listing, ats)
		} else if fetcher != nil {
			doc, err := fetcher.Fetch(ctx, jobURL)
			if err != nil {
				listing.Live = false
				listing.DeadReason = "could not retrieve the posting"
				if jdText == "" {
					return listing, nil
				}
			} else if doc != nil {
				if listing.Text == "" {
					listing.Text = strings.TrimSpace(doc.Text)
				}
				if listing.Title == "" {
					listing.Title = strings.TrimSpace(doc.Title)
				}
				if IsDeadText(doc.Text) {
					listing.Live = false
					listing.DeadReason = "the posting looks expired or closed"
				}
			}
		}
	}
	if listing.Text == "" && jdText != "" {
		listing.Text = jdText
	}
	if listing.Text != "" && IsDeadText(listing.Text) {
		listing.Live = false
		if listing.DeadReason == "" {
			listing.DeadReason = "the posting looks expired or closed"
		}
	}
	if listing.Title == "" {
		listing.Title = firstHeading(listing.Text)
	}
	if listing.Company == "" {
		listing.Company = guessCompanyFromText(listing.Text)
	}
	if listing.Via == "web" {
		listing.Via, listing.Agency = inferVia(listing.Text, jobURL)
	}
	return listing, nil
}

func IsDeadText(text string) bool {
	lower := strings.ToLower(text)
	for _, p := range deadPhrases {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

func tryATSJSON(ctx context.Context, httpc *http.Client, jobURL string) *JobListing {
	if httpc == nil {
		httpc = http.DefaultClient
	}
	if m := greenhouseJob.FindStringSubmatch(jobURL); len(m) == 3 {
		api := fmt.Sprintf("https://boards-api.greenhouse.io/v1/boards/%s/jobs/%s", m[1], m[2])
		return fetchGreenhouseJob(ctx, httpc, api, jobURL, m[1])
	}
	if m := leverJob.FindStringSubmatch(jobURL); len(m) == 3 {
		api := fmt.Sprintf("https://api.lever.co/v0/postings/%s/%s", m[1], m[2])
		return fetchLeverJob(ctx, httpc, api, jobURL, m[1])
	}
	if m := ashbyJob.FindStringSubmatch(jobURL); len(m) == 3 {
		// Ashby job-board API is list-shaped; fall through to page fetch.
		return nil
	}
	return nil
}

func fetchGreenhouseJob(ctx context.Context, httpc *http.Client, api, pageURL, slug string) *JobListing {
	var body struct {
		Title   string `json:"title"`
		Company struct {
			Name string `json:"name"`
		} `json:"company"`
		Content  string `json:"content"`
		Absolute string `json:"absolute_url"`
	}
	if err := getJSON(ctx, httpc, api, &body); err != nil {
		return nil
	}
	text := stripHTML(body.Content)
	if text == "" {
		return nil
	}
	company := body.Company.Name
	if company == "" {
		company = slug
	}
	u := pageURL
	if body.Absolute != "" {
		u = body.Absolute
	}
	return &JobListing{URL: u, Company: company, Title: body.Title, Text: text, Via: "greenhouse", Live: true}
}

func fetchLeverJob(ctx context.Context, httpc *http.Client, api, pageURL, slug string) *JobListing {
	var body struct {
		Text       string `json:"text"`
		Categories struct {
			Team string `json:"team"`
		} `json:"categories"`
		DescriptionPlain string `json:"descriptionPlain"`
		HostedURL        string `json:"hostedUrl"`
	}
	if err := getJSON(ctx, httpc, api, &body); err != nil {
		return nil
	}
	text := strings.TrimSpace(body.DescriptionPlain)
	if text == "" {
		text = strings.TrimSpace(body.Text)
	}
	if text == "" {
		return nil
	}
	u := pageURL
	if body.HostedURL != "" {
		u = body.HostedURL
	}
	return &JobListing{URL: u, Company: slug, Title: body.Text, Text: text, Via: "lever", Live: true}
}

func getJSON(ctx context.Context, httpc *http.Client, rawURL string, dest any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "JobShout-CareerOps/1.0")
	client := httpc
	if client.Timeout == 0 {
		c := *httpc
		c.Timeout = 15 * time.Second
		client = &c
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("not found")
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("http %d", resp.StatusCode)
	}
	limited := io.LimitReader(resp.Body, 2<<20)
	return json.NewDecoder(limited).Decode(dest)
}

func mergeListing(dst, src *JobListing) {
	if src == nil {
		return
	}
	if src.Text != "" {
		dst.Text = src.Text
	}
	if src.Title != "" {
		dst.Title = src.Title
	}
	if src.Company != "" {
		dst.Company = src.Company
	}
	if src.Via != "" {
		dst.Via = src.Via
	}
	if src.URL != "" {
		dst.URL = src.URL
	}
	dst.Live = src.Live
	dst.DeadReason = src.DeadReason
}

func guessCompanyFromHost(host string) string {
	host = strings.ToLower(host)
	host = strings.TrimPrefix(host, "www.")
	// boards.greenhouse.io, jobs.lever.co, jobs.ashbyhq.com are ATS hosts.
	if strings.Contains(host, "greenhouse") || strings.Contains(host, "lever.co") || strings.Contains(host, "ashby") {
		return ""
	}
	parts := strings.Split(host, ".")
	if len(parts) >= 2 {
		return parts[0]
	}
	return host
}

func guessCompanyFromText(text string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "company:") {
			return strings.TrimSpace(line[len("company:"):])
		}
		if strings.HasPrefix(lower, "about ") && len(line) < 80 {
			return strings.TrimSpace(line[len("about "):])
		}
	}
	return ""
}

func firstHeading(text string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.TrimLeft(line, "#"))
		line = strings.TrimSpace(line)
		if line != "" && len(line) < 160 {
			return line
		}
	}
	return ""
}

func inferVia(text, jobURL string) (via, agency string) {
	lower := strings.ToLower(text + " " + jobURL)
	switch {
	case strings.Contains(lower, "greenhouse"):
		return "greenhouse", ""
	case strings.Contains(lower, "ashby"):
		return "ashby", ""
	case strings.Contains(lower, "lever.co"):
		return "lever", ""
	case strings.Contains(lower, "recruiter") || strings.Contains(lower, "on behalf of") || strings.Contains(lower, "contingent"):
		return "agency", "agency"
	default:
		return "web", ""
	}
}

var htmlTag = regexp.MustCompile(`(?s)<[^>]*>`)

func stripHTML(s string) string {
	// Greenhouse (and others) sometimes entity-escape the HTML, so unescape
	// before stripping tags or the JD stays as "&lt;h2&gt;Who we are".
	s = html.UnescapeString(s)
	s = strings.ReplaceAll(s, "<br>", "\n")
	s = strings.ReplaceAll(s, "<br/>", "\n")
	s = strings.ReplaceAll(s, "<p>", "\n")
	s = htmlTag.ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}
