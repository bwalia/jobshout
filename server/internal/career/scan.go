package career

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// PostedJob is one ATS listing before it becomes a pipeline item.
type PostedJob struct {
	URL     string
	Company string
	Title   string
	Board   string
}

// ScanBoard lists public Greenhouse / Ashby / Lever jobs. Zero-LLM.
func ScanBoard(ctx context.Context, httpc *http.Client, board, slug, company string) ([]PostedJob, error) {
	board = strings.ToLower(strings.TrimSpace(board))
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return nil, fmt.Errorf("career: board slug is required")
	}
	if httpc == nil {
		httpc = http.DefaultClient
	}
	switch board {
	case "greenhouse":
		return scanGreenhouse(ctx, httpc, slug, company)
	case "ashby":
		return scanAshby(ctx, httpc, slug, company)
	case "lever":
		return scanLever(ctx, httpc, slug, company)
	default:
		return nil, fmt.Errorf("career: unknown board %q (use greenhouse, ashby, or lever)", board)
	}
}

func scanGreenhouse(ctx context.Context, httpc *http.Client, slug, company string) ([]PostedJob, error) {
	var body struct {
		Jobs []struct {
			Title       string `json:"title"`
			AbsoluteURL string `json:"absolute_url"`
		} `json:"jobs"`
	}
	api := fmt.Sprintf("https://boards-api.greenhouse.io/v1/boards/%s/jobs", slug)
	if err := getJSON(ctx, httpc, api, &body); err != nil {
		return nil, fmt.Errorf("career: greenhouse: %w", err)
	}
	if company == "" {
		company = slug
	}
	out := make([]PostedJob, 0, len(body.Jobs))
	for _, j := range body.Jobs {
		if j.AbsoluteURL == "" {
			continue
		}
		out = append(out, PostedJob{URL: j.AbsoluteURL, Company: company, Title: j.Title, Board: "greenhouse"})
	}
	return out, nil
}

func scanAshby(ctx context.Context, httpc *http.Client, slug, company string) ([]PostedJob, error) {
	var body struct {
		Jobs []struct {
			Title       string `json:"title"`
			JobURL      string `json:"jobUrl"`
			AbsoluteURL string `json:"absoluteUrl"`
		} `json:"jobs"`
	}
	api := fmt.Sprintf("https://api.ashbyhq.com/posting-api/job-board/%s", slug)
	if err := getJSON(ctx, httpc, api, &body); err != nil {
		return nil, fmt.Errorf("career: ashby: %w", err)
	}
	if company == "" {
		company = slug
	}
	out := make([]PostedJob, 0, len(body.Jobs))
	for _, j := range body.Jobs {
		u := j.JobURL
		if u == "" {
			u = j.AbsoluteURL
		}
		if u == "" {
			continue
		}
		out = append(out, PostedJob{URL: u, Company: company, Title: j.Title, Board: "ashby"})
	}
	return out, nil
}

func scanLever(ctx context.Context, httpc *http.Client, slug, company string) ([]PostedJob, error) {
	var jobs []struct {
		Text      string `json:"text"`
		HostedURL string `json:"hostedUrl"`
	}
	api := fmt.Sprintf("https://api.lever.co/v0/postings/%s", slug)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, api, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := httpc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("career: lever: http %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&jobs); err != nil {
		return nil, err
	}
	if company == "" {
		company = slug
	}
	out := make([]PostedJob, 0, len(jobs))
	for _, j := range jobs {
		if j.HostedURL == "" {
			continue
		}
		out = append(out, PostedJob{URL: j.HostedURL, Company: company, Title: j.Text, Board: "lever"})
	}
	return out, nil
}
