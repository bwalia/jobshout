package marketingemail

import (
	"net/url"
	"strings"
)

// siteByOpsAPIHost maps an OpsAPI host to the website that serves its pages.
// A table, not a string hack: adding a ring or a site is adding a row.
var siteByOpsAPIHost = map[string]string{
	"int-opsapi.workstation.co.uk": "https://int.workstation.co.uk",
	"opsapi.workstation.co.uk":     "https://www.workstation.co.uk",
}

// BrowserURL is where an email's "view in your browser" version is served:
// <site>/email/<slug>. The site comes from siteURLOverride (a destination's
// config.site_url) when it is a valid https URL, else from the OpsAPI host.
// It returns "" when the site cannot be worked out — never a guess.
func BrowserURL(opsapiBase, slug, siteURLOverride string) string {
	slug = strings.Trim(strings.TrimSpace(slug), "/")
	if slug == "" {
		return ""
	}
	site := cleanSite(siteURLOverride)
	if site == "" {
		if u, err := url.Parse(strings.TrimSpace(opsapiBase)); err == nil {
			site = siteByOpsAPIHost[strings.ToLower(u.Hostname())]
		}
	}
	if site == "" {
		return ""
	}
	return site + "/email/" + url.PathEscape(slug)
}

// cleanSite returns an https site root without a trailing slash, or "".
func cleanSite(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return ""
	}
	return "https://" + strings.ToLower(u.Host) + strings.TrimRight(u.Path, "/")
}
