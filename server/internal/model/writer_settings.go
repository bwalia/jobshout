package model

import "strings"

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
)

// WriterSettings is a writer agent's configuration, read from EngineConfig.
type WriterSettings struct {
	Style       string
	Audience    string
	Industry    string
	Focus       []string
	Destination string
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
	}
}

// ApplyWriterSettings fills what the request left unset from the agent's
// settings. What a launch or schedule names itself always wins, so one agent
// can serve several schedules with different keywords.
func (r *GenerateBlogRequest) ApplyWriterSettings(ws WriterSettings) {
	if r.Style == "" {
		r.Style = ws.Style
	}
	if r.Audience == "" {
		r.Audience = ws.Audience
	}
	if r.Industry == "" {
		r.Industry = ws.Industry
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
