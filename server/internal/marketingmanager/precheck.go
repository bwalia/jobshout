package marketingmanager

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// ErrTestRecipient is a test send to an address nobody confirmed as a tester.
var ErrTestRecipient = errors.New("test recipient not confirmed")

// maxImportRows matches OpsAPI's own per-import limit.
const maxImportRows = 2000

// precheck enforces what OpsAPI leaves to the caller, before anything is
// sent to it. done=true means res already holds the answer (an existing
// contact) and OpsAPI must not be called.
func (s *Service) precheck(ctx context.Context, c *Client, op Op, call *Call, res *Result) (bool, error) {
	switch op.Name {
	case "test_send":
		return false, s.checkTestRecipient(ctx, c, call)
	case "create_contact", "update_contact":
		if call.Body == nil {
			return false, nil
		}
		if raw, ok := call.Body["email"]; ok && str(raw) != "" {
			e, valid := ValidEmail(str(raw))
			if !valid {
				return false, fmt.Errorf("%q is not a valid email address", str(raw))
			}
			call.Body["email"] = e
			if op.Name == "create_contact" {
				existing, err := c.findContact(ctx, e)
				if err != nil {
					return false, err
				}
				if existing != nil {
					res.Data = existing
					res.Note = "A contact with this email already exists in this workspace; it was not created again."
					return true, nil
				}
			}
		}
	case "import_contacts":
		return false, checkImport(str(call.Body["csv"]))
	case "block_address":
		if e, ok := ValidEmail(str(call.Body["address"])); ok && str(call.Body["channel"]) != "sms" {
			call.Body["address"] = e
		}
	}
	return false, nil
}

// checkTestRecipient allows a test only to a confirmed tester: OpsAPI's
// test-send accepts any address (and creates a CRM contact for it).
func (s *Service) checkTestRecipient(ctx context.Context, c *Client, call *Call) error {
	allowed := c.conn.TestRecipients
	if len(allowed) == 0 {
		return fmt.Errorf("%w: no test recipients are confirmed for workspace %q. An admin adds them to the website integration's config.marketing_test_recipients (or MARKETING_TEST_RECIPIENTS on the server)", ErrTestRecipient, c.Namespace())
	}
	to := str(call.Body["email"])
	if to == "" {
		cu := str(call.Body["contact_uuid"])
		if cu == "" {
			return errors.New("test_send needs body.email (a confirmed test recipient)")
		}
		if !idRe.MatchString(cu) {
			return fmt.Errorf("%q is not a valid contact uuid", cu)
		}
		resp, err := c.Do(ctx, http.MethodGet, crmContacts+"/"+cu, nil, nil, "")
		if err != nil {
			return err
		}
		var ct struct {
			Email string `json:"email"`
		}
		_ = json.Unmarshal(resp.Data, &ct)
		to = ct.Email
	}
	e, ok := ValidEmail(to)
	if !ok {
		return fmt.Errorf("%w: %q is not a valid email address", ErrTestRecipient, to)
	}
	for _, a := range allowed {
		if a == e {
			if str(call.Body["contact_uuid"]) == "" {
				call.Body["email"] = e
			}
			return nil
		}
	}
	return fmt.Errorf("%w: %s is not a confirmed test recipient for %q (confirmed: %s)", ErrTestRecipient, e, c.Namespace(), strings.Join(allowed, ", "))
}

// findContact returns the CRM contact with exactly this email, if any.
func (c *Client) findContact(ctx context.Context, email string) (json.RawMessage, error) {
	resp, err := c.Do(ctx, http.MethodGet, crmContacts, url.Values{"search": {email}, "per_page": {"20"}}, nil, "")
	if err != nil {
		return nil, fmt.Errorf("checking for an existing contact: %w", err)
	}
	var items []json.RawMessage
	if json.Unmarshal(resp.Data, &items) != nil {
		return nil, nil
	}
	for _, it := range items {
		var ct struct {
			Email string `json:"email"`
		}
		if json.Unmarshal(it, &ct) == nil && strings.EqualFold(strings.TrimSpace(ct.Email), email) {
			return it, nil
		}
	}
	return nil, nil
}

// checkImport refuses a CSV OpsAPI would reject or half-import: no email
// column, too many rows, or invalid addresses (listed so they can be fixed).
func checkImport(raw string) error {
	r := csv.NewReader(strings.NewReader(strings.TrimSpace(raw)))
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	r.TrimLeadingSpace = true
	rows, err := r.ReadAll()
	if err != nil {
		return fmt.Errorf("body.csv is not valid CSV: %v", err)
	}
	if len(rows) < 2 {
		return errors.New("import_contacts needs body.csv: a header row (email,first_name,last_name,phone) and at least one contact")
	}
	if len(rows)-1 > maxImportRows {
		return fmt.Errorf("the CSV has %d rows; OpsAPI imports at most %d at a time — split it", len(rows)-1, maxImportRows)
	}
	col := -1
	for i, h := range rows[0] {
		if strings.EqualFold(strings.TrimSpace(h), "email") {
			col = i
		}
	}
	if col < 0 {
		return errors.New("the CSV header has no email column")
	}
	var bad []string
	for i, row := range rows[1:] {
		v := ""
		if col < len(row) {
			v = row[col]
		}
		if _, ok := ValidEmail(v); !ok {
			bad = append(bad, fmt.Sprintf("row %d %q", i+2, v))
		}
	}
	if len(bad) > 0 {
		n := len(bad)
		if n > 10 {
			bad = append(bad[:10], fmt.Sprintf("and %d more", n-10))
		}
		return fmt.Errorf("%d invalid email address(es); fix or remove them first: %s", n, strings.Join(bad, ", "))
	}
	return nil
}
