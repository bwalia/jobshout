package marketingemail

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"html/template"
	"os"
	"regexp"
	"strings"

	"github.com/jobshout/server/internal/model"
)

//go:embed templates/email.html.tmpl
var emailTemplateSrc string

var emailTemplate = template.Must(template.New("email").Parse(emailTemplateSrc))

// Environment knobs for the email chrome.
const (
	EnvLogoURL        = "MARKETING_EMAIL_LOGO_URL"
	EnvUnsubscribeURL = "MARKETING_EMAIL_UNSUBSCRIBE_URL"
	EnvPreferencesURL = "MARKETING_EMAIL_PREFERENCES_URL"
	EnvCompanyName    = "MARKETING_EMAIL_COMPANY"
	EnvCompanyAddress = "MARKETING_EMAIL_ADDRESS"
	EnvSiteURL        = "MARKETING_EMAIL_HOME_URL"
)

// Defaults for the Workstation brand. DefaultLogoURL is only a candidate: the
// service checks it answers 200 before using it (see LogoChecker), and the
// header falls back to a text wordmark otherwise — never a broken image.
const (
	DefaultLogoURL        = "https://www.workstation.co.uk/img/email/workstation-logo.png"
	DefaultUnsubscribeURL = "https://www.workstation.co.uk/en/contact-us"
	DefaultCompanyName    = "Workstation"
	DefaultHomeURL        = "https://www.workstation.co.uk"
)

// Chrome is the fixed furniture around an email: logo, footer, links.
// An empty LogoURL renders the text wordmark instead of an image.
type Chrome struct {
	LogoURL        string
	UnsubscribeURL string
	PreferencesURL string
	CompanyName    string
	CompanyAddress string
	HomeURL        string
}

// ChromeFromEnv reads the chrome from the environment, falling back to the
// Workstation defaults. A value that is not https is ignored.
func ChromeFromEnv() Chrome {
	unsub := httpsOr(os.Getenv(EnvUnsubscribeURL), DefaultUnsubscribeURL)
	return Chrome{
		LogoURL:        httpsOr(os.Getenv(EnvLogoURL), DefaultLogoURL),
		UnsubscribeURL: unsub,
		PreferencesURL: httpsOr(os.Getenv(EnvPreferencesURL), unsub),
		CompanyName:    firstNonEmpty(os.Getenv(EnvCompanyName), DefaultCompanyName),
		CompanyAddress: strings.TrimSpace(os.Getenv(EnvCompanyAddress)),
		HomeURL:        httpsOr(os.Getenv(EnvSiteURL), DefaultHomeURL),
	}
}

func httpsOr(v, def string) string {
	if v = strings.TrimSpace(v); IsHTTPS(v) {
		return v
	}
	return def
}

func (c Chrome) withDefaults() Chrome {
	c.LogoURL = httpsOr(c.LogoURL, "")
	if c.LogoURL != "" && !safeHref(c.LogoURL) {
		c.LogoURL = ""
	}
	c.UnsubscribeURL = httpsOr(c.UnsubscribeURL, DefaultUnsubscribeURL)
	c.PreferencesURL = httpsOr(c.PreferencesURL, c.UnsubscribeURL)
	c.HomeURL = httpsOr(c.HomeURL, DefaultHomeURL)
	c.CompanyName = firstNonEmpty(c.CompanyName, DefaultCompanyName)
	return c
}

// Merge fields the mail-merge version uses. OpsAPI Marketing refuses to
// publish a template naming any key outside its registry; these are all in it.
var AllowedMergeFields = []string{
	"contact.first_name", "contact.last_name", "contact.email", "contact.company",
	"unsubscribe_url", "preferences_url", "view_online_url",
}

// Sentinels stand in for merge tags while html/template renders (it would
// percent-encode "{{" inside an href), and are swapped for the tags after.
// stripMarkers removes the sentinel prefix from model text, so the model can
// never produce one.
const (
	sentinelPrefix    = "jsmergetag"
	sentinelFirstName = sentinelPrefix + "firstname"
	sentinelUnsub     = "https://" + sentinelPrefix + ".invalid/unsubscribe_url"
	sentinelPrefs     = "https://" + sentinelPrefix + ".invalid/preferences_url"
	sentinelView      = "https://" + sentinelPrefix + ".invalid/view_online_url"
)

var mergeTags = strings.NewReplacer(
	sentinelUnsub, "{{unsubscribe_url}}",
	sentinelPrefs, "{{preferences_url}}",
	sentinelView, "{{view_online_url}}",
	sentinelFirstName, "{{contact.first_name}}",
)

// greetingDefault is what {{contact.first_name}} falls back to in OpsAPI, and
// what the browser version shows in its place.
const greetingDefault = "there"

var mergeKeyRe = regexp.MustCompile(`\{\{\s*([A-Za-z0-9_.]+)\s*\}\}`)

// MergeKeys lists the {{key}} merge fields in s, in order of first use.
func MergeKeys(s string) []string {
	var out []string
	seen := map[string]bool{}
	for _, m := range mergeKeyRe.FindAllStringSubmatch(s, -1) {
		if !seen[m[1]] {
			seen[m[1]] = true
			out = append(out, m[1])
		}
	}
	return out
}

type sectionView struct {
	IsText, IsFeature, IsProduct, IsQuote, IsCTA bool
	Heading                                      string
	Body                                         template.HTML
	Items                                        []template.HTML
	CTALabel, CTAURL                             string
}

type pageView struct {
	Subject, Preheader, Headline, Subline, HeroImageURL, FooterNote string
	Chrome
	// Wordmark is the header text when there is no logo image.
	Wordmark string
	// Greeting is the first name (or its default) after "Hi".
	Greeting string
	// ViewOnlineURL is set only in the mail-merge version: the browser
	// version needs no "view in your browser" line.
	ViewOnlineURL string
	Sections      []sectionView
}

// Render turns a validated draft into the browser version: the full HTML email
// document the CMS page serves. heroImageURL is used only when it is an
// absolute https URL; otherwise there is no image. Every piece of model text
// is escaped (html/template) or goes through the markdown allowlist. Merge
// fields are rendered as their defaults ("Hi there,", the configured footer
// links), so the result never contains a "{{" merge tag.
func Render(d *model.MarketingEmailDraft, heroImageURL string, chrome Chrome) (string, error) {
	out, err := render(d, heroImageURL, chrome, false)
	if err != nil {
		return "", err
	}
	if strings.Contains(out, "{{") {
		return "", errors.New("marketingemail: render: output still contains a {{ merge tag")
	}
	return out, nil
}

// RenderMailMerge renders the same email for OpsAPI Marketing's mail merge:
// {{contact.first_name}} in the greeting, {{unsubscribe_url}} and
// {{preferences_url}} in the footer, and a top line linking
// {{view_online_url}}. It uses no merge field outside AllowedMergeFields.
func RenderMailMerge(d *model.MarketingEmailDraft, heroImageURL string, chrome Chrome) (string, error) {
	out, err := render(d, heroImageURL, chrome, true)
	if err != nil {
		return "", err
	}
	if err := checkMergeKeys(out); err != nil {
		return "", err
	}
	return out, nil
}

func checkMergeKeys(s string) error {
	allowed := map[string]bool{}
	for _, k := range AllowedMergeFields {
		allowed[k] = true
	}
	for _, k := range MergeKeys(s) {
		if !allowed[k] {
			return fmt.Errorf("marketingemail: render: merge field {{%s}} is not allowed", k)
		}
	}
	if strings.Contains(s, sentinelPrefix) {
		return errors.New("marketingemail: render: a merge placeholder was left in the output")
	}
	return nil
}

func render(d *model.MarketingEmailDraft, heroImageURL string, chrome Chrome, merge bool) (string, error) {
	if d == nil {
		return "", errors.New("marketingemail: render: no draft")
	}
	chrome = chrome.withDefaults()
	if !IsHTTPS(heroImageURL) || !safeHref(heroImageURL) {
		heroImageURL = ""
	}

	view := pageView{
		Subject:      stripMarkers(strings.TrimSpace(d.Subject)),
		Preheader:    stripMarkers(strings.TrimSpace(d.Preheader)),
		Headline:     stripMarkers(strings.TrimSpace(d.Hero.Headline)),
		Subline:      stripMarkers(strings.TrimSpace(d.Hero.Subline)),
		HeroImageURL: heroImageURL,
		FooterNote:   stripMarkers(strings.TrimSpace(d.FooterNote)),
		Chrome:       chrome,
		Wordmark:     strings.ToUpper(chrome.CompanyName),
		Greeting:     greetingDefault,
	}
	if merge {
		view.Greeting = sentinelFirstName
		view.UnsubscribeURL = sentinelUnsub
		view.PreferencesURL = sentinelPrefs
		view.ViewOnlineURL = sentinelView
	}
	for _, s := range d.Sections {
		sv := sectionView{
			Heading:  stripMarkers(strings.TrimSpace(s.Heading)),
			Body:     MarkdownHTML(s.BodyMarkdown),
			CTALabel: stripMarkers(strings.TrimSpace(s.CTALabel)),
		}
		if u := strings.TrimSpace(s.CTAURL); IsHTTPS(u) && safeHref(u) {
			sv.CTAURL = u
		}
		if sv.CTAURL == "" {
			sv.CTALabel = "" // a button without a destination is not rendered
		}
		for _, it := range s.Items {
			if it = strings.TrimSpace(it); it != "" {
				sv.Items = append(sv.Items, template.HTML(inlineHTML(it))) // #nosec G203 — escaped by inlineHTML
			}
		}
		switch s.Type {
		case model.EmailSectionFeature:
			sv.IsFeature = true
		case model.EmailSectionProduct:
			sv.IsProduct = true
		case model.EmailSectionQuote:
			sv.IsQuote = true
		case model.EmailSectionCTA:
			sv.IsCTA = true
		default:
			sv.IsText = true
		}
		view.Sections = append(view.Sections, sv)
	}

	var buf bytes.Buffer
	if err := emailTemplate.Execute(&buf, view); err != nil {
		return "", fmt.Errorf("marketingemail: render: %w", err)
	}
	out := buf.String()
	if merge {
		out = mergeTags.Replace(out)
	}
	return out, nil
}
