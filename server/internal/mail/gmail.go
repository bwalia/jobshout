package mail

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	netmail "net/mail"
	"net/url"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
)

const gmailAPI = "https://gmail.googleapis.com/gmail/v1/users/me"

// GmailAPI is the Gmail surface the Mail Agent needs. Tests substitute a fake.
type GmailAPI interface {
	ExchangeCode(ctx context.Context, code, redirectURL, clientID, clientSecret string) (TokenSet, error)
	Refresh(ctx context.Context, refreshToken, clientID, clientSecret string) (TokenSet, error)
	Profile(ctx context.Context, accessToken string) (string, error)
	ListMessages(ctx context.Context, accessToken, query string, limit int) ([]InboxMessage, error)
	Send(ctx context.Context, accessToken string, msg OutboundMessage) (string, error)
}

type httpGmail struct {
	client *http.Client
	logger *zap.Logger
}

// NewGmailAPI talks to Google over HTTP. No official client library: the
// surface is small and tests inject a fake instead.
func NewGmailAPI(client *http.Client, logger *zap.Logger) GmailAPI {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &httpGmail{client: client, logger: logger}
}

func (g *httpGmail) ExchangeCode(ctx context.Context, code, redirectURL, clientID, clientSecret string) (TokenSet, error) {
	body, status, err := postForm(ctx, g.client, googleTokenURL, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURL},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
	})
	if err != nil {
		return TokenSet{}, err
	}
	return parseTokenResponse(body, status)
}

func (g *httpGmail) Refresh(ctx context.Context, refreshToken, clientID, clientSecret string) (TokenSet, error) {
	body, status, err := postForm(ctx, g.client, googleTokenURL, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
	})
	if err != nil {
		return TokenSet{}, err
	}
	ts, err := parseTokenResponse(body, status)
	if err != nil {
		return TokenSet{}, err
	}
	if ts.RefreshToken == "" {
		ts.RefreshToken = refreshToken
	}
	return ts, nil
}

func (g *httpGmail) Profile(ctx context.Context, accessToken string) (string, error) {
	var p struct {
		EmailAddress string `json:"emailAddress"`
	}
	if err := g.getJSON(ctx, accessToken, gmailAPI+"/profile", &p); err != nil {
		return "", err
	}
	if p.EmailAddress == "" {
		return "", fmt.Errorf("mail: gmail profile had no email")
	}
	return p.EmailAddress, nil
}

func (g *httpGmail) ListMessages(ctx context.Context, accessToken, query string, limit int) ([]InboxMessage, error) {
	if limit <= 0 {
		limit = 25
	}
	if limit > 50 {
		limit = 50
	}
	var list struct {
		Messages []struct {
			ID       string `json:"id"`
			ThreadID string `json:"threadId"`
		} `json:"messages"`
	}
	q := url.Values{"maxResults": {fmt.Sprintf("%d", limit)}}
	if query != "" {
		q.Set("q", query)
	}
	if err := g.getJSON(ctx, accessToken, gmailAPI+"/messages?"+q.Encode(), &list); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	out := make([]InboxMessage, 0, len(list.Messages))
	for _, m := range list.Messages {
		if m.ThreadID == "" || seen[m.ThreadID] {
			continue
		}
		seen[m.ThreadID] = true
		msg, err := g.getMessage(ctx, accessToken, m.ID)
		if err != nil {
			// One unreadable message must not fail the whole inbox sync.
			g.logger.Warn("mail: skipping gmail message",
				zap.String("gmail_message_id", m.ID),
				zap.String("gmail_thread_id", m.ThreadID),
				zap.Error(RedactErr(err)))
			continue
		}
		out = append(out, msg)
	}
	return out, nil
}

func (g *httpGmail) Send(ctx context.Context, accessToken string, msg OutboundMessage) (string, error) {
	raw := encodeRFC822(msg)
	payload, _ := json.Marshal(map[string]string{
		"raw":      raw,
		"threadId": msg.ThreadID,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, gmailAPI+"/messages/send", strings.NewReader(string(payload)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := g.client.Do(req)
	if err != nil {
		return "", RedactErr(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("mail: gmail send: %w", gmailAPIStatusError(resp.StatusCode, body))
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &out); err != nil || out.ID == "" {
		return "", fmt.Errorf("mail: gmail send: missing message id")
	}
	return out.ID, nil
}

func (g *httpGmail) getMessage(ctx context.Context, accessToken, id string) (InboxMessage, error) {
	var raw gmailMessage
	if err := g.getJSON(ctx, accessToken, gmailAPI+"/messages/"+url.PathEscape(id)+"?format=full", &raw); err != nil {
		return InboxMessage{}, err
	}
	return parseGmailMessage(raw), nil
}

func (g *httpGmail) getJSON(ctx context.Context, accessToken, rawURL string, v any) error {
	var last error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			delay := time.Duration(1<<uint(attempt-1)) * time.Second
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}
		err := g.doGetJSON(ctx, accessToken, rawURL, v)
		if err == nil {
			return nil
		}
		if !gmailRetryable(err) {
			return err
		}
		last = err
	}
	return last
}

func (g *httpGmail) doGetJSON(ctx context.Context, accessToken, rawURL string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := g.client.Do(req)
	if err != nil {
		return RedactErr(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode >= 400 {
		return gmailAPIStatusError(resp.StatusCode, body)
	}
	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("mail: gmail api: decode (%v)", err)
	}
	return nil
}

type gmailStatusError struct {
	status int
	msg    string
}

func (e gmailStatusError) Error() string {
	if e.msg != "" {
		return fmt.Sprintf("mail: gmail api: status %d: %s", e.status, e.msg)
	}
	return fmt.Sprintf("mail: gmail api: status %d", e.status)
}

func gmailRetryable(err error) bool {
	var ge gmailStatusError
	if errors.As(err, &ge) {
		return ge.status == http.StatusTooManyRequests || ge.status == http.StatusServiceUnavailable
	}
	return false
}

func gmailAPIStatusError(status int, body []byte) error {
	var ge struct {
		Error struct {
			Message string `json:"message"`
			Status  string `json:"status"`
		} `json:"error"`
	}
	msg := ""
	if json.Unmarshal(body, &ge) == nil {
		msg = strings.TrimSpace(ge.Error.Message)
	}
	return gmailStatusError{status: status, msg: Redact(msg)}
}

type gmailMessage struct {
	ID       string       `json:"id"`
	ThreadID string       `json:"threadId"`
	Snippet  string       `json:"snippet"`
	Internal gmailEpochMS `json:"internalDate"`
	Payload  gmailPart    `json:"payload"`
}

// gmailEpochMS accepts Gmail's internalDate as either a JSON number or a
// string. Google documents it as milliseconds since epoch and often encodes
// it as a string so JavaScript does not lose precision.
type gmailEpochMS struct{ ms int64 }

func (e *gmailEpochMS) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(strings.Trim(string(b), `"`))
	if s == "" || s == "null" {
		return nil
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return err
	}
	e.ms = n
	return nil
}

type gmailPart struct {
	MimeType string `json:"mimeType"`
	Headers  []struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	} `json:"headers"`
	Body struct {
		Data string `json:"data"`
	} `json:"body"`
	Parts []gmailPart `json:"parts"`
}

func parseGmailMessage(raw gmailMessage) InboxMessage {
	h := func(name string) string {
		for _, hdr := range raw.Payload.Headers {
			if strings.EqualFold(hdr.Name, name) {
				return hdr.Value
			}
		}
		return ""
	}
	fromEmail, fromName := splitAddress(h("From"))
	toEmail, _ := splitAddress(h("To"))
	msg := InboxMessage{
		GmailThreadID:    raw.ThreadID,
		GmailMessageID:   raw.ID,
		FromEmail:        fromEmail,
		FromName:         fromName,
		ToEmail:          toEmail,
		Subject:          h("Subject"),
		Snippet:          raw.Snippet,
		Body:             extractText(raw.Payload),
		MessageIDHeader:  h("Message-Id"),
		ReferencesHeader: h("References"),
	}
	if raw.Internal.ms > 0 {
		msg.ReceivedAt = time.UnixMilli(raw.Internal.ms)
	}
	return msg
}

func extractText(p gmailPart) string {
	if strings.HasPrefix(strings.ToLower(p.MimeType), "text/plain") && p.Body.Data != "" {
		return decodeBody(p.Body.Data)
	}
	var html string
	for _, part := range p.Parts {
		if t := extractText(part); t != "" {
			if strings.HasPrefix(strings.ToLower(part.MimeType), "text/plain") || strings.HasPrefix(strings.ToLower(p.MimeType), "multipart/") {
				if !strings.HasPrefix(strings.ToLower(part.MimeType), "text/html") {
					return t
				}
			}
			html = t
		}
	}
	if p.Body.Data != "" && html == "" {
		html = decodeBody(p.Body.Data)
	}
	if strings.Contains(strings.ToLower(p.MimeType), "html") || html != "" {
		return stripTags(html)
	}
	return html
}

func decodeBody(data string) string {
	data = strings.ReplaceAll(data, "-", "+")
	data = strings.ReplaceAll(data, "_", "/")
	b, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		if b2, err2 := base64.RawStdEncoding.DecodeString(data); err2 == nil {
			return string(b2)
		}
		return ""
	}
	return string(b)
}

func stripTags(s string) string {
	var b strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
			b.WriteByte(' ')
		case !inTag:
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

func splitAddress(raw string) (email, name string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ""
	}
	a, err := netmail.ParseAddress(raw)
	if err != nil {
		return raw, ""
	}
	return a.Address, a.Name
}

func encodeRFC822(msg OutboundMessage) string {
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", msg.From)
	fmt.Fprintf(&b, "To: %s\r\n", msg.To)
	if strings.TrimSpace(msg.CC) != "" {
		fmt.Fprintf(&b, "Cc: %s\r\n", msg.CC)
	}
	fmt.Fprintf(&b, "Subject: %s\r\n", msg.Subject)
	if msg.InReplyTo != "" {
		fmt.Fprintf(&b, "In-Reply-To: %s\r\n", msg.InReplyTo)
	}
	if msg.References != "" {
		fmt.Fprintf(&b, "References: %s\r\n", msg.References)
	}
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n\r\n")
	b.WriteString(msg.Body)
	return base64.RawURLEncoding.EncodeToString([]byte(b.String()))
}

// SearchQuery builds a Gmail search string from watch rules.
func SearchQuery(rules ...string) string {
	// Recent inbox only — already-ingested threads are skipped in the
	// service layer, so is:unread is not required and hides mail the
	// operator already opened in Gmail (the usual "Sync now did nothing"
	// case). newer_than:7d still caps a first connect.
	q := "in:inbox newer_than:7d"
	var extra []string
	for _, r := range rules {
		r = strings.TrimSpace(r)
		if r != "" {
			extra = append(extra, r)
		}
	}
	if len(extra) == 0 {
		return q
	}
	return q + " " + strings.Join(extra, " ")
}

// gmailOp formats a Gmail search operator. Values with spaces or reserved
// characters must be quoted — `from:Balinder Walia` is parsed as
// `from:Balinder` AND the word Walia, which matches nothing.
func gmailOp(op, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = strings.ReplaceAll(value, `"`, "")
	if strings.ContainsAny(value, " \t(){}[]:'") {
		return op + `:"` + value + `"`
	}
	return op + ":" + value
}

// RulesQuery maps stored watch rules onto Gmail search operators.
func RulesQuery(labels, senders, prefixes []string) string {
	var parts []string
	for _, s := range senders {
		if t := gmailOp("from", s); t != "" {
			parts = append(parts, t)
		}
	}
	for _, l := range labels {
		if t := gmailOp("label", l); t != "" {
			parts = append(parts, t)
		}
	}
	for _, p := range prefixes {
		if t := gmailOp("subject", p); t != "" {
			parts = append(parts, t)
		}
	}
	if len(parts) == 0 {
		return SearchQuery()
	}
	return SearchQuery("(" + strings.Join(parts, " OR ") + ")")
}

func compactWatch(xs []string) []string {
	out := make([]string, 0, len(xs))
	for _, x := range xs {
		if t := strings.TrimSpace(x); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func hasWatchRule(labels, senders, prefixes []string) bool {
	return len(compactWatch(labels)) > 0 || len(compactWatch(senders)) > 0 || len(compactWatch(prefixes)) > 0
}

// WatchMatches is true when the operator asked to watch this mail (sender,
// subject prefix, or Gmail label). Matching follows Gmail search: `from:` and
// `subject:` are contains, not exact prefix. Labels are applied in the Gmail
// query, so a labels-only playbook treats every ingested message as a match.
func WatchMatches(msg InboxMessage, labels, senders, prefixes []string) bool {
	senders = compactWatch(senders)
	prefixes = compactWatch(prefixes)
	if !hasWatchRule(labels, senders, prefixes) {
		return false
	}
	from := strings.ToLower(strings.TrimSpace(msg.FromName + " " + msg.FromEmail))
	for _, s := range senders {
		if strings.Contains(from, strings.ToLower(s)) {
			return true
		}
	}
	subj := strings.ToLower(strings.TrimSpace(msg.Subject))
	for _, p := range prefixes {
		if strings.Contains(subj, strings.ToLower(p)) {
			return true
		}
	}
	if len(senders) == 0 && len(prefixes) == 0 {
		return true
	}
	return false
}

// AppendWatchSender adds this mail's sender to the watch list so later
// triage will not ignore them. Prefers the email address; falls back to the
// display name. Does not add subject prefixes — those are too easy to guess
// wrong. Empty candidate or a case-insensitive duplicate is a no-op.
func AppendWatchSender(senders []string, fromEmail, fromName string) (out []string, added string) {
	candidate := strings.TrimSpace(fromEmail)
	if candidate == "" {
		candidate = strings.TrimSpace(fromName)
	}
	out = make([]string, len(senders))
	copy(out, senders)
	if candidate == "" {
		return out, ""
	}
	for _, s := range out {
		if strings.EqualFold(strings.TrimSpace(s), candidate) {
			return out, ""
		}
	}
	return append(out, candidate), candidate
}

// HonorOperatorWatch keeps triage from discarding mail the operator
// explicitly watched. OTP / no-reply / "notification" heuristics otherwise
// mark those threads ignored after a successful Gmail match.
func HonorOperatorWatch(c ClassifyResult) ClassifyResult {
	if c.SuggestedAction != "ignore" {
		return c
	}
	c.SuggestedAction = "reply"
	if c.Intent == "spam" {
		c.Intent = "request"
	}
	c.Reason = "Operator watch rule matched; do not ignore."
	return c
}
