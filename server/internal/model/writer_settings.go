package model

import (
	"strings"

	"github.com/google/uuid"
)

// Writer settings are EngineConfig keys on a writer agent (the Article Writer
// or a clone of it). They are how one writer is configured for a website or
// project without code: clone the agent, set these, point a schedule at it.
// Agent packs carry them, so a configuration tested on int is promoted to the
// other rings by export and import.
const (
	// EngineConfigWriterStyle is free-text writing-style instructions: tone,
	// length, what every piece must include.
	EngineConfigWriterStyle = "writer_style"
	// EngineConfigWriterAudience is the default audience.Profile key.
	EngineConfigWriterAudience = "writer_audience"
	// EngineConfigWriterIndustry is the default sector framing.
	EngineConfigWriterIndustry = "writer_industry"
	// EngineConfigWriterFocus is the default focus keywords for a trending run:
	// a list of strings, or one comma-separated string.
	EngineConfigWriterFocus = "writer_focus"
	// EngineConfigWriterDestination names the org's opsapi integration the
	// writer files to — "Workstation Website". A name rather than an ID, so a
	// config exported from one ring finds that ring's own integration (and its
	// own key) on import. Empty is the ring's CMS.
	EngineConfigWriterDestination = "writer_destination"
	// EngineConfigWriterRules is free-text rules every article must keep —
	// what a regulated site may never say ("no investment advice", "never
	// promise a return"). Unlike the style, the self-review checks the draft
	// against them and the revision fixes every break.
	EngineConfigWriterRules = "writer_rules"
	// EngineConfigWriterAuthor is the byline on the website's posts. Empty is
	// the ring's BLOG_AUTHOR_NAME.
	EngineConfigWriterAuthor = "writer_author"
	// EngineConfigWriterCategories is the website categories a post is filed
	// under, by name or slug: a list, or one comma-separated string. They must
	// already exist on the website; the first is the post's primary category.
	EngineConfigWriterCategories = "writer_categories"
	// EngineConfigWriterTags replaces the tags a post gets by default (who it
	// was written for, and its industry) with these: a list, or one
	// comma-separated string. Public sites show tags to readers.
	EngineConfigWriterTags = "writer_tags"
	// EngineConfigWriterCMSImages, when true, uploads a post's cover and the
	// images in its body to the website's own image storage (OpsAPI
	// /api/v2/cms/media), so the site does not load them from this ring.
	EngineConfigWriterCMSImages = "writer_cms_images"
)

// WriterSettings is a writer agent's configuration, read from EngineConfig.
type WriterSettings struct {
	Style       string
	Audience    string
	Industry    string
	Focus       []string
	Destination string
	Rules       string
	// Post is how the writer's articles are filed on its website.
	Post PostSettings
}

// PostSettings is how a writer files posts on its website. The zero value
// files them as every writer did before these settings existed.
type PostSettings struct {
	Author     string   `json:"author,omitempty"`
	Categories []string `json:"categories,omitempty"`
	Tags       []string `json:"tags,omitempty"`
	CMSImages  bool     `json:"cms_images,omitempty"`
}

// WriterSettingsOf reads a writer agent's settings. A nil agent, or one never
// configured, has none — and runs exactly as before.
func WriterSettingsOf(a *Agent) WriterSettings {
	if a == nil || a.EngineConfig == nil {
		return WriterSettings{}
	}
	str := func(k string) string {
		v, _ := a.EngineConfig[k].(string)
		return strings.TrimSpace(v)
	}
	return WriterSettings{
		Style:       str(EngineConfigWriterStyle),
		Audience:    str(EngineConfigWriterAudience),
		Industry:    str(EngineConfigWriterIndustry),
		Focus:       stringList(a.EngineConfig[EngineConfigWriterFocus]),
		Destination: str(EngineConfigWriterDestination),
		Rules:       str(EngineConfigWriterRules),
		Post: PostSettings{
			Author:     str(EngineConfigWriterAuthor),
			Categories: stringList(a.EngineConfig[EngineConfigWriterCategories]),
			Tags:       stringList(a.EngineConfig[EngineConfigWriterTags]),
			CMSImages:  boolSetting(a.EngineConfig[EngineConfigWriterCMSImages]),
		},
	}
}

// boolSetting reads a JSON boolean, or the strings "true"/"false" a form or
// an older pack may have saved.
func boolSetting(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return strings.EqualFold(strings.TrimSpace(t), "true")
	}
	return false
}

// ApplyWriterSettings fills what the request left unset from the agent's
// settings. What a launch or schedule names itself always wins, so one agent
// can serve several schedules with different keywords.
func (r *GenerateBlogRequest) ApplyWriterSettings(ws WriterSettings) {
	if r.Style == "" {
		r.Style = ws.Style
	}
	if r.Rules == "" {
		r.Rules = ws.Rules
	}
	if r.Audience == "" {
		r.Audience = ws.Audience
	}
	if r.Industry == "" {
		r.Industry = ws.Industry
	}
	if r.Destination == "" {
		r.Destination = ws.Destination
	}
	if r.Trending && len(r.Focus) == 0 && len(ws.Focus) > 0 {
		r.Focus = append([]string(nil), ws.Focus...)
	}
}

// stringList reads a JSON list of strings, or a comma-separated string, as
// trimmed non-empty strings.
func stringList(v any) []string {
	var raw []string
	switch t := v.(type) {
	case []string:
		raw = t
	case []any:
		for _, x := range t {
			if s, ok := x.(string); ok {
				raw = append(raw, s)
			}
		}
	case string:
		raw = strings.Split(t, ",")
	}
	var out []string
	for _, s := range raw {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// BlogDestination is a website a writer can file to, as the UI sees it: never
// the key, only whether one is configured.
type BlogDestination struct {
	Name      string `json:"name"`
	BaseURL   string `json:"base_url"`
	Namespace string `json:"namespace"`
	// Ready is false when the integration is missing its key, namespace or a
	// valid URL, or is not active — filing to it would fail.
	Ready bool `json:"ready"`
}

// WebsiteCheck answers the add-website form's first question: is this
// namespace (on this OpsAPI) already a saved website of the org? It never
// carries a key — Ready only says whether the stored one is usable.
type WebsiteCheck struct {
	Namespace string `json:"namespace"`
	BaseURL   string `json:"base_url"`
	Exists    bool   `json:"exists"`
	// Name, IntegrationID and Ready describe the saved website when Exists.
	Name          string     `json:"name,omitempty"`
	IntegrationID *uuid.UUID `json:"integration_id,omitempty"`
	Ready         bool       `json:"ready"`
	// Ring is the ring's own CMS namespace: it exists, with the ring's
	// configured key, without being saved as a website.
	Ring bool `json:"ring,omitempty"`
}
