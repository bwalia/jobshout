package marketingemail

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/integration/adapters/opsapi"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/secretbox"
)

// Pages is the slice of the opsapi CMS client the service uses. *opsapi.Client
// satisfies it; tests use a fake.
type Pages interface {
	CreatePage(ctx context.Context, req opsapi.CreatePageRequest) (*opsapi.Page, error)
	UpdatePage(ctx context.Context, pageUUID string, fields any) (*opsapi.Page, error)
	FindPageBySlug(ctx context.Context, slug string) (*opsapi.Page, error)
}

// IntegrationLister reads an org's integrations.
type IntegrationLister interface {
	ListByOrg(ctx context.Context, orgID uuid.UUID) ([]model.Integration, error)
}

// Destination is a resolved website: the CMS client plus what is needed to
// work out the email's public browser URL. It never exposes the key.
type Destination struct {
	Pages     Pages
	BaseURL   string // OpsAPI root, for BrowserURL's host map
	Namespace string
	// SiteURL is the integration's config.site_url override, if any.
	SiteURL string
}

// BrowserURL is the public browser-version URL of slug on this destination.
func (d *Destination) BrowserURL(slug string) string {
	return BrowserURL(d.BaseURL, slug, d.SiteURL)
}

// DestinationResolver finds the CMS an email files to.
type DestinationResolver interface {
	Resolve(ctx context.Context, orgID uuid.UUID, name string) (*Destination, error)
}

// Destinations resolves a website name against the org's saved opsapi
// integrations, falling back to the ring's own CMS (OPSAPI_BASE_URL / KEY /
// NAMESPACE). It mirrors the Article Writer's rule — see
// service/blog_destination.go — kept local so this package does not import
// the service layer.
type Destinations struct {
	Integrations IntegrationLister
	// Ring is the ring's own CMS client, nil when not configured.
	Ring *opsapi.Client
}

var errNoDestination = errors.New("marketingemail: no such website")

// ErrDestination marks a launch naming a website this org cannot file to.
var ErrDestination = errors.New("marketingemail: destination")

// Resolve returns the CMS for the website called name: an opsapi integration
// by name, else one saved for that namespace, else the ring's own CMS (for an
// empty name, or the ring's namespace). A named website that cannot be
// resolved is an error: never quietly file to another site.
func (d *Destinations) Resolve(ctx context.Context, orgID uuid.UUID, name string) (*Destination, error) {
	name = strings.TrimSpace(name)
	if name != "" {
		var list []model.Integration
		if d != nil && d.Integrations != nil {
			var err error
			if list, err = d.Integrations.ListByOrg(ctx, orgID); err != nil {
				return nil, fmt.Errorf("marketingemail: list integrations: %w", err)
			}
		}
		ringBase := ""
		if d != nil && d.Ring != nil {
			ringBase = d.Ring.BaseURL()
		}
		dest, err := findDestination(list, name, ringBase)
		if err == nil {
			return dest, nil
		}
		if !errors.Is(err, errNoDestination) {
			return nil, fmt.Errorf("%w: %v", ErrDestination, err)
		}
		if !d.isRingNamespace(name) {
			return nil, fmt.Errorf("%w: %v", ErrDestination, err)
		}
	}
	if d == nil || d.Ring == nil {
		return nil, fmt.Errorf("%w: no CMS is configured on this server (set OPSAPI_BASE_URL, OPSAPI_API_KEY and OPSAPI_NAMESPACE) and no website was named", ErrDestination)
	}
	return &Destination{Pages: d.Ring, BaseURL: d.Ring.BaseURL(), Namespace: d.Ring.Namespace()}, nil
}

func (d *Destinations) isRingNamespace(name string) bool {
	return d != nil && d.Ring != nil && strings.EqualFold(strings.TrimSpace(name), d.Ring.Namespace())
}

// findDestination picks the opsapi website a run names: the integration
// called name (case-insensitively), else the one saved for namespace name —
// on ringBase when the namespace is saved on several OpsAPIs.
func findDestination(list []model.Integration, name, ringBase string) (*Destination, error) {
	for _, in := range list {
		if strings.EqualFold(strings.TrimSpace(in.Name), name) {
			return integrationDestination(in)
		}
	}
	var matches []model.Integration
	for _, in := range list {
		if in.Provider != model.ProviderOpsAPI {
			continue
		}
		ns, _ := in.Config["namespace_slug"].(string)
		if !strings.EqualFold(strings.TrimSpace(ns), name) {
			continue
		}
		if ringBase != "" && normalBase(in.BaseURL) == normalBase(ringBase) {
			return integrationDestination(in)
		}
		matches = append(matches, in)
	}
	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("%w: emails file to %q, but this organization has no website of that name or namespace", errNoDestination, name)
	case 1:
		return integrationDestination(matches[0])
	}
	return nil, fmt.Errorf("namespace %q is saved on %d OpsAPIs — pick the website by name", name, len(matches))
}

// integrationDestination builds the CMS client for a named opsapi integration:
//
//	base_url              the opsapi API root
//	credentials.api_key   a namespace-scoped key with cms create/update/read
//	config.namespace_slug the namespace that owns the site's pages
//	config.site_url       optional: the public site, overriding the host map
func integrationDestination(in model.Integration) (*Destination, error) {
	if in.Provider != model.ProviderOpsAPI {
		return nil, fmt.Errorf("integration %q is a %s integration, not opsapi", in.Name, in.Provider)
	}
	if in.Status != "" && in.Status != "active" {
		return nil, fmt.Errorf("integration %q is %s", in.Name, in.Status)
	}
	base := strings.TrimRight(strings.TrimSpace(in.BaseURL), "/")
	if u, err := url.Parse(base); err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return nil, fmt.Errorf("integration %q has no valid base_url", in.Name)
	}
	key, _ := in.Credentials["api_key"].(string)
	if strings.TrimSpace(key) == "" {
		return nil, fmt.Errorf("integration %q has no credentials.api_key", in.Name)
	}
	if secretbox.IsSealed(key) {
		return nil, fmt.Errorf("integration %q has a key this server cannot decrypt (check SECRETS_ENCRYPTION_KEY)", in.Name)
	}
	ns, _ := in.Config["namespace_slug"].(string)
	ns = strings.TrimSpace(ns)
	if ns == "" {
		return nil, fmt.Errorf("integration %q has no config.namespace_slug", in.Name)
	}
	client := opsapi.NewClient(opsapi.Config{BaseURL: base, APIKey: strings.TrimSpace(key), Namespace: ns})
	if client == nil {
		return nil, fmt.Errorf("integration %q is incomplete", in.Name)
	}
	site, _ := in.Config["site_url"].(string)
	return &Destination{Pages: client, BaseURL: base, Namespace: ns, SiteURL: site}, nil
}

func normalBase(raw string) string {
	b := strings.TrimRight(strings.TrimSpace(raw), "/")
	u, err := url.Parse(b)
	if err != nil || u.Host == "" {
		return strings.ToLower(b)
	}
	return strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host) + u.Path
}
