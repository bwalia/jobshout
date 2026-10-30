package marketingmanager

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/secretbox"
)

// IntegrationLister reads an org's integrations (credentials decrypted).
// Satisfied by repository.IntegrationRepository.
type IntegrationLister interface {
	ListByOrg(ctx context.Context, orgID uuid.UUID) ([]model.Integration, error)
}

// RingConfig is this ring's own OpsAPI workspace, used when an org names no
// website. Key is a marketing-scoped key (OPSAPI_MARKETING_API_KEY), never the
// CMS key: the CMS key has no marketing scopes, and should not get them.
type RingConfig struct {
	BaseURL   string
	Key       string
	Namespace string
	// TestRecipients (MARKETING_TEST_RECIPIENTS) are the confirmed addresses
	// test sends may go to, for every workspace on this ring.
	TestRecipients []string
	// Orgs (OPSAPI_MARKETING_ORG_IDS) are the only organizations that may use
	// this workspace. Empty means none: the ring's customers and campaigns are
	// never offered to every org that signs up.
	Orgs []uuid.UUID
}

func (r RingConfig) complete() bool {
	return strings.TrimSpace(r.BaseURL) != "" && strings.TrimSpace(r.Key) != "" && strings.TrimSpace(r.Namespace) != ""
}

// allows reports whether org may use the ring's workspace.
func (r RingConfig) allows(org uuid.UUID) bool {
	if !r.complete() || org == uuid.Nil {
		return false
	}
	for _, o := range r.Orgs {
		if o == org {
			return true
		}
	}
	return false
}

// ErrNotConfigured means the org has no OpsAPI workspace the agent can use.
var ErrNotConfigured = errors.New("marketingmanager: not configured")

// resolve picks the OpsAPI workspace for website:
//
//   - an opsapi integration named website (case-insensitive), else one saved
//     for that namespace slug;
//   - empty website: the org's only opsapi integration that carries a
//     marketing key, else the ring's own workspace if the org is one of
//     ring.Orgs.
//
// A named website that cannot be found is an error: the agent never quietly
// manages a different tenant's marketing.
func resolve(ctx context.Context, list IntegrationLister, ring RingConfig, orgID uuid.UUID, website string) (Conn, error) {
	website = strings.TrimSpace(website)
	var ints []model.Integration
	if list != nil {
		all, err := list.ListByOrg(ctx, orgID)
		if err != nil {
			return Conn{}, fmt.Errorf("list integrations: %w", err)
		}
		for _, in := range all {
			if in.Provider == model.ProviderOpsAPI && (in.Status == "" || in.Status == "active") {
				ints = append(ints, in)
			}
		}
	}

	if website != "" {
		for _, in := range ints {
			if strings.EqualFold(strings.TrimSpace(in.Name), website) {
				return connFrom(in)
			}
		}
		var byNS []model.Integration
		for _, in := range ints {
			if strings.EqualFold(strings.TrimSpace(configString(in, "namespace_slug")), website) {
				byNS = append(byNS, in)
			}
		}
		switch len(byNS) {
		case 1:
			return connFrom(byNS[0])
		case 0:
		default:
			return Conn{}, fmt.Errorf("%w: namespace %q is saved on %d OpsAPIs — name the website instead", ErrNotConfigured, website, len(byNS))
		}
		if ring.allows(orgID) && strings.EqualFold(website, ring.Namespace) {
			return ringConn(ring), nil
		}
		return Conn{}, fmt.Errorf("%w: this organization has no OpsAPI website called %q", ErrNotConfigured, website)
	}

	var marketing []model.Integration
	for _, in := range ints {
		if credString(in, "marketing_api_key") != "" {
			marketing = append(marketing, in)
		}
	}
	if len(marketing) == 1 {
		return connFrom(marketing[0])
	}
	if len(marketing) > 1 {
		names := make([]string, 0, len(marketing))
		for _, in := range marketing {
			names = append(names, in.Name)
		}
		return Conn{}, fmt.Errorf("%w: several websites have a marketing key (%s) — say which one", ErrNotConfigured, strings.Join(names, ", "))
	}
	if ring.allows(orgID) {
		return ringConn(ring), nil
	}
	if ring.complete() {
		return Conn{}, fmt.Errorf("%w: this organization has no OpsAPI marketing workspace. Add credentials.marketing_api_key to its OpsAPI website integration (a key scoped for opsapi_marketing_* and crm); the server's own workspace is only for organizations listed in OPSAPI_MARKETING_ORG_IDS", ErrNotConfigured)
	}
	return Conn{}, fmt.Errorf("%w: no OpsAPI marketing workspace is set up. Add credentials.marketing_api_key to the org's OpsAPI website integration (a key scoped for opsapi_marketing_* and crm), or set OPSAPI_MARKETING_API_KEY and OPSAPI_MARKETING_ORG_IDS on this server", ErrNotConfigured)
}

func ringConn(r RingConfig) Conn {
	return Conn{
		Website:        "ring",
		BaseURL:        strings.TrimRight(strings.TrimSpace(r.BaseURL), "/"),
		APIKey:         strings.TrimSpace(r.Key),
		Namespace:      strings.TrimSpace(r.Namespace),
		TestRecipients: normEmails(r.TestRecipients),
	}
}

// connFrom reads an opsapi integration. credentials.marketing_api_key is
// preferred so the website's CMS key need not be widened; api_key is the
// fallback for an org that minted one key for both.
func connFrom(in model.Integration) (Conn, error) {
	base := strings.TrimRight(strings.TrimSpace(in.BaseURL), "/")
	if u, err := url.Parse(base); err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return Conn{}, fmt.Errorf("%w: website %q has no valid base_url", ErrNotConfigured, in.Name)
	}
	key := credString(in, "marketing_api_key")
	if key == "" {
		key = credString(in, "api_key")
	}
	if key == "" {
		return Conn{}, fmt.Errorf("%w: website %q has no API key", ErrNotConfigured, in.Name)
	}
	if secretbox.IsSealed(key) {
		return Conn{}, fmt.Errorf("%w: website %q has a key this server cannot decrypt (check SECRETS_ENCRYPTION_KEY)", ErrNotConfigured, in.Name)
	}
	ns := strings.TrimSpace(configString(in, "namespace_slug"))
	if ns == "" {
		return Conn{}, fmt.Errorf("%w: website %q has no config.namespace_slug", ErrNotConfigured, in.Name)
	}
	brand, _ := in.Config["marketing_brand"].(map[string]any)
	return Conn{
		Website: in.Name, BaseURL: base, APIKey: key, Namespace: ns,
		TestRecipients: normEmails(listValue(in.Config["marketing_test_recipients"])),
		Brand:          brand,
	}, nil
}

// withRing adds the ring's confirmed test recipients to a website's own.
func withRing(c Conn, r RingConfig) Conn {
	c.TestRecipients = normEmails(append(append([]string{}, c.TestRecipients...), r.TestRecipients...))
	return c
}

// listValue reads a config value saved as a JSON array or a comma list.
func listValue(v any) []string {
	switch t := v.(type) {
	case string:
		return strings.Split(t, ",")
	case []any:
		out := make([]string, 0, len(t))
		for _, x := range t {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return t
	}
	return nil
}

// normEmails keeps the valid addresses, lower case, once each.
func normEmails(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		e, ok := ValidEmail(s)
		if ok && !seen[e] {
			seen[e] = true
			out = append(out, e)
		}
	}
	return out
}

func credString(in model.Integration, k string) string {
	s, _ := in.Credentials[k].(string)
	return strings.TrimSpace(s)
}

func configString(in model.Integration, k string) string {
	s, _ := in.Config[k].(string)
	return s
}
