package llm

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// qwen3-coder (and other Hermes-template coder models) sometimes emit their
// internal tool-call markup as plain assistant text instead of a structured
// tool_calls message. Ollama cannot parse the malformed variant, so it passes
// the markup through as content — which the chat UI would then show verbatim.
// Recover it: parse the markup back into ToolCalls and strip it from the text.
//
// Two shapes are recovered:
//
//	<function=image_generate>
//	<parameter=prompt>
//	a tiger doing bhangra
//	</parameter>
//	</function>
//
//	<tool_call>
//	{"name": "image_generate", "arguments": {"prompt": "a tiger"}}
//	</tool_call>
var (
	leakedFuncRe  = regexp.MustCompile(`(?s)<function=([a-zA-Z0-9_.:-]+)>(.*?)</function>`)
	leakedParamRe = regexp.MustCompile(`(?s)<parameter=([a-zA-Z0-9_.:-]+)>(.*?)</parameter>`)
	leakedCallRe  = regexp.MustCompile(`(?s)<tool_call>\s*(\{.*?\})\s*</tool_call>`)
)

// leakMarkers are the openings of the shapes above. The stream guard uses
// them to stop forwarding tokens the moment leaked markup starts.
var leakMarkers = []string{"<function=", "<tool_call>"}

// recoverLeakedToolCalls parses leaked tool-call markup out of text. It
// returns the recovered calls, the text with every leaked block removed
// (including an unclosed trailing one — the model may have been cut off
// mid-markup), and whether anything was recovered.
func recoverLeakedToolCalls(text string) ([]ToolCall, string, bool) {
	var calls []ToolCall
	for _, m := range leakedFuncRe.FindAllStringSubmatch(text, -1) {
		args := map[string]any{}
		for _, pm := range leakedParamRe.FindAllStringSubmatch(m[2], -1) {
			args[pm[1]] = coerceLeakedValue(pm[2])
		}
		calls = append(calls, ToolCall{Name: m[1], Arguments: args})
	}
	for _, m := range leakedCallRe.FindAllStringSubmatch(text, -1) {
		var payload struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if json.Unmarshal([]byte(m[1]), &payload) == nil && payload.Name != "" {
			if payload.Arguments == nil {
				payload.Arguments = map[string]any{}
			}
			calls = append(calls, ToolCall{Name: payload.Name, Arguments: payload.Arguments})
		}
	}
	if len(calls) == 0 {
		return nil, text, false
	}
	cleaned := leakedFuncRe.ReplaceAllString(text, "")
	cleaned = leakedCallRe.ReplaceAllString(cleaned, "")
	for _, mark := range leakMarkers {
		if i := strings.Index(cleaned, mark); i >= 0 {
			cleaned = cleaned[:i]
		}
	}
	for i := range calls {
		calls[i].ID = fmt.Sprintf("call_%d", i)
	}
	return calls, strings.TrimSpace(cleaned), true
}

// coerceLeakedValue turns a markup parameter body into a JSON-ish value:
// objects, arrays, numbers and booleans keep their type, everything else is
// the trimmed string.
func coerceLeakedValue(raw string) any {
	v := strings.TrimSpace(raw)
	var out any
	if err := json.Unmarshal([]byte(v), &out); err == nil {
		switch out.(type) {
		case map[string]any, []any, float64, bool, string:
			return out
		}
	}
	return v
}

// leakStreamGuard forwards streamed content tokens but stops the moment
// leaked tool-call markup begins, so the markup never reaches a client
// mid-stream. A trailing run that could be the start of a marker is held
// back until the next chunk resolves it.
type leakStreamGuard struct {
	onToken    func(string)
	pending    strings.Builder
	suppressed bool
}

func (g *leakStreamGuard) feed(s string) {
	if g == nil || g.onToken == nil || s == "" || g.suppressed {
		return
	}
	g.pending.WriteString(s)
	pending := g.pending.String()

	cut := -1
	for _, m := range leakMarkers {
		if i := strings.Index(pending, m); i >= 0 && (cut == -1 || i < cut) {
			cut = i
		}
	}
	if cut >= 0 {
		if cut > 0 {
			g.onToken(pending[:cut])
		}
		g.suppressed = true
		g.pending.Reset()
		return
	}

	hold := 0
	for _, m := range leakMarkers {
		max := len(m) - 1
		if max > len(pending) {
			max = len(pending)
		}
		for l := max; l > 0; l-- {
			if strings.HasSuffix(pending, m[:l]) {
				if l > hold {
					hold = l
				}
				break
			}
		}
	}
	if emit := len(pending) - hold; emit > 0 {
		g.onToken(pending[:emit])
		rest := pending[emit:]
		g.pending.Reset()
		g.pending.WriteString(rest)
	}
}
