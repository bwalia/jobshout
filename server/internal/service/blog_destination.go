package service

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/jobshout/server/internal/blog"
	"github.com/jobshout/server/internal/integration/adapters/opsapi"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/secretbox"
)

// CMSDestination is a website a writer agent files articles in.
type CMSDestination = blog.CMSPublisher

// opsapiDestination builds the CMS client for a named opsapi integration:
//
//	base_url              the opsapi API root, e.g. https://opsapi.workstation.co.uk
//	credentials.api_key   a namespace-scoped key with cms create/update/read
//	config.namespace_slug the namespace that owns the site's posts
//
// The key stays on the integration — server-side, per ring — and the agent
// refers to the integration only by name.
func opsapiDestination(in model.Integration) (CMSDestination, error) {
	if in.Provider != model.ProviderOpsAPI {
		return nil, fmt.Errorf("blog_svc: integration %q is a %s integration, not opsapi", in.Name, in.Provider)
	}
	if in.Status != "" && in.Status != "active" {
		return nil, fmt.Errorf("blog_svc: integration %q is %s", in.Name, in.Status)
	}
	base := strings.TrimRight(strings.TrimSpace(in.BaseURL), "/")
	if u, err := url.Parse(base); err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return nil, fmt.Errorf("blog_svc: integration %q has no valid base_url", in.Name)
	}
	key, _ := in.Credentials["api_key"].(string)
	if strings.TrimSpace(key) == "" {
		return nil, fmt.Errorf("blog_svc: integration %q has no credentials.api_key", in.Name)
	}
	// Still sealed after reading: this server has no or another
	// SECRETS_ENCRYPTION_KEY, and sending the ciphertext would only fail auth.
	if secretbox.IsSealed(key) {
		return nil, fmt.Errorf("blog_svc: integration %q has a key this server cannot decrypt (check SECRETS_ENCRYPTION_KEY)", in.Name)
	}
	ns, _ := in.Config["namespace_slug"].(string)
	if strings.TrimSpace(ns) == "" {
		return nil, fmt.Errorf("blog_svc: integration %q has no config.namespace_slug", in.Name)
	}
	return opsapi.NewClient(opsapi.Config{
		BaseURL:   base,
		APIKey:    strings.TrimSpace(key),
		Namespace: strings.TrimSpace(ns),
	}), nil
}

// normalBaseURL is an OpsAPI root as websites are compared: no trailing
// slash, case-insensitive scheme and host.
func normalBaseURL(raw string) string {
	b := strings.TrimRight(strings.TrimSpace(raw), "/")
	u, err := url.Parse(b)
	if err != nil || u.Host == "" {
		return strings.ToLower(b)
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	return u.String()
}

// sameWebsite reports whether in is the opsapi website for namespace on
// baseURL. One namespace on one OpsAPI is one website, with one stored key.
func sameWebsite(in model.Integration, namespace, baseURL string) bool {
	if in.Provider != model.ProviderOpsAPI {
		return false
	}
	ns, _ := in.Config["namespace_slug"].(string)
	return strings.EqualFold(strings.TrimSpace(ns), strings.TrimSpace(namespace)) &&
		normalBaseURL(in.BaseURL) == normalBaseURL(baseURL)
}
