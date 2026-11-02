package metalead

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/marketingmanager"
	"github.com/jobshout/server/internal/model"
)

// ErrCRMNotConfigured means the org has no OpsAPI CRM workspace the agent can use.
var ErrCRMNotConfigured = errors.New("meta lead CRM: OpsAPI marketing/CRM workspace not configured")

// CRMContact is the shape we push into OpsAPI /api/v2/crm/contacts.
type CRMContact struct {
	FirstName string
	LastName  string
	Email     string
	Phone     string
	JobTitle  string
	// LeadgenID is used only for logging / idempotency notes.
	LeadgenID string
}

// CRMResult is what a successful Upsert returned.
type CRMResult struct {
	ContactID string
	Website   string
	Existed   bool // true when OpsAPI already had this email
}

// CRMSync upserts an analysed Meta lead into OpsAPI CRM contacts.
// Implemented by wrapping the Marketing Manager (same workspace resolution).
type CRMSync interface {
	UpsertContact(ctx context.Context, orgID uuid.UUID, website string, c CRMContact) (CRMResult, error)
	ConfiguredFor(ctx context.Context, orgID uuid.UUID, website string) bool
}

// MarketingCRM pushes contacts via marketingmanager.Service (OpsAPI CRM).
type MarketingCRM struct {
	MM *marketingmanager.Service
}

// UpsertContact creates (or returns the existing) OpsAPI CRM contact.
func (m *MarketingCRM) UpsertContact(ctx context.Context, orgID uuid.UUID, website string, c CRMContact) (CRMResult, error) {
	if m == nil || m.MM == nil {
		return CRMResult{}, ErrCRMNotConfigured
	}
	first := strings.TrimSpace(c.FirstName)
	if first == "" {
		first = "Meta Lead"
	}
	body := map[string]any{"first_name": first}
	if v := strings.TrimSpace(c.LastName); v != "" {
		body["last_name"] = v
	}
	if v := strings.TrimSpace(c.Email); v != "" {
		body["email"] = v
	}
	if v := strings.TrimSpace(c.Phone); v != "" {
		body["phone"] = v
	}
	if v := strings.TrimSpace(c.JobTitle); v != "" {
		body["job_title"] = truncateRunes(v, 120)
	}
	res, err := m.MM.Run(ctx, orgID, marketingmanager.Write, marketingmanager.Call{
		Action:  "create_contact",
		Website: strings.TrimSpace(website),
		Body:    body,
	})
	if err != nil {
		if errors.Is(err, marketingmanager.ErrNotConfigured) {
			return CRMResult{}, fmt.Errorf("%w: %v", ErrCRMNotConfigured, err)
		}
		return CRMResult{}, err
	}
	id := contactIDFromData(res.Data)
	if id == "" {
		return CRMResult{}, fmt.Errorf("OpsAPI create_contact returned no contact uuid")
	}
	return CRMResult{
		ContactID: id,
		Website:   res.Website,
		Existed:   strings.Contains(strings.ToLower(res.Note), "already exists"),
	}, nil
}

// ConfiguredFor reports whether the org can reach an OpsAPI CRM workspace.
func (m *MarketingCRM) ConfiguredFor(ctx context.Context, orgID uuid.UUID, website string) bool {
	if m == nil || m.MM == nil {
		return false
	}
	_, err := m.MM.Connect(ctx, orgID, strings.TrimSpace(website))
	return err == nil
}

func contactIDFromData(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var one struct {
		UUID string `json:"uuid"`
		ID   string `json:"id"`
	}
	if json.Unmarshal(raw, &one) == nil {
		if one.UUID != "" {
			return one.UUID
		}
		if one.ID != "" {
			return one.ID
		}
	}
	return ""
}

// ContactFromFields maps Instant Form field_data (+ brief) into a CRM contact.
func ContactFromFields(fields []FieldData, brief *model.MetaLeadBrief, leadgenID string) CRMContact {
	m := fieldMap(fields)
	email := firstField(m, "email", "email_address", "work_email", "business_email")
	phone := firstField(m, "phone", "phone_number", "mobile", "mobile_phone", "work_phone")
	firstName := firstField(m, "first_name", "firstname", "given_name")
	lastName := firstField(m, "last_name", "lastname", "family_name", "surname")
	if full := firstField(m, "full_name", "name", "full name"); full != "" && firstName == "" {
		parts := strings.Fields(full)
		if len(parts) > 0 {
			firstName = parts[0]
		}
		if len(parts) > 1 {
			lastName = strings.Join(parts[1:], " ")
		}
	}
	if firstName == "" {
		if email != "" {
			firstName = strings.SplitN(email, "@", 2)[0]
		} else {
			firstName = "Meta Lead"
		}
	}
	job := firstField(m, "job_title", "job", "role", "company_name", "company")
	if brief != nil && brief.FitScore > 0 {
		tag := fmt.Sprintf("Meta lead fit %d", brief.FitScore)
		if job == "" {
			job = tag
		} else {
			job = truncateRunes(job+" · "+tag, 120)
		}
	}
	return CRMContact{
		FirstName: firstName,
		LastName:  lastName,
		Email:     email,
		Phone:     phone,
		JobTitle:  job,
		LeadgenID: leadgenID,
	}
}

func fieldMap(fields []FieldData) map[string]string {
	out := make(map[string]string, len(fields))
	for _, f := range fields {
		key := strings.ToLower(strings.TrimSpace(f.Name))
		key = strings.ReplaceAll(key, " ", "_")
		if key == "" || len(f.Values) == 0 {
			continue
		}
		v := strings.TrimSpace(f.Values[0])
		if v != "" {
			out[key] = v
		}
	}
	return out
}

func firstField(m map[string]string, keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(m[k]); v != "" {
			return v
		}
	}
	return ""
}

func truncateRunes(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	return s[:n]
}

// ShouldPushCRM is false for high spam risk unless force is set.
func ShouldPushCRM(brief *model.MetaLeadBrief, force bool) bool {
	if force {
		return true
	}
	if brief == nil {
		return true
	}
	return !strings.EqualFold(strings.TrimSpace(brief.SpamRisk), "high")
}
