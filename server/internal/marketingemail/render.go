package marketingemail

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"html/template"
	"os"
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
	EnvCompanyName    = "MARKETING_EMAIL_COMPANY"
	EnvCompanyAddress = "MARKETING_EMAIL_ADDRESS"
	EnvSiteURL        = "MARKETING_EMAIL_HOME_URL"
)

// Defaults for the Workstation brand.
const (
	DefaultLogoURL        = "https://www.workstation.co.uk/img/email/workstation-logo.png"
	DefaultUnsubscribeURL = "https://www.workstation.co.uk/en/contact-us"
	DefaultCompanyName    = "Workstation"
	DefaultHomeURL        = "https://www.workstation.co.uk"
)

// Chrome is the fixed furniture around an email: logo, footer, links.
type Chrome struct {
	LogoURL        string
	UnsubscribeURL string
	CompanyName    string
	CompanyAddress string
	HomeURL        string
}

// ChromeFromEnv reads the chrome from the environment, falling back to the
// Workstation defaults. A value that is not https is ignored.
func ChromeFromEnv() Chrome {
	return Chrome{
		LogoURL:        httpsOr(os.Getenv(EnvLogoURL), DefaultLogoURL),
		UnsubscribeURL: httpsOr(os.Getenv(EnvUnsubscribeURL), DefaultUnsubscribeURL),
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
	c.LogoURL = httpsOr(c.LogoURL, DefaultLogoURL)
	c.UnsubscribeURL = httpsOr(c.UnsubscribeURL, DefaultUnsubscribeURL)
	c.HomeURL = httpsOr(c.HomeURL, DefaultHomeURL)
	c.CompanyName = firstNonEmpty(c.CompanyName, DefaultCompanyName)
	return c
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
	Sections []sectionView
}

// Render turns a validated draft into the full HTML email document. heroImageURL
// is used only when it is an absolute https URL; otherwise there is no image.
// Every piece of model text is escaped (html/template) or goes through the
// markdown allowlist; the result never contains a "{{" merge tag.
func Render(d *model.MarketingEmailDraft, heroImageURL string, chrome Chrome) (string, error) {
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
	if strings.Contains(out, "{{") {
		return "", errors.New("marketingemail: render: output still contains a {{ merge tag")
	}
	return out, nil
}
