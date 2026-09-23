package research

import "testing"

var intFocus = []string{"Observability for AI Agents and LLMs", "SRE", "AWS AgentCore", "AWS X-Ray", "Grafana", "Prometheus", "Thanos"}

func TestConfirmFocus(t *testing.T) {
	cases := []struct {
		name, topic, context string
		want                 bool
	}{
		// The int run that prompted this: shares "AI agents", nothing else.
		{"adjacent vocabulary only", "Enabling Secure AI Agents with HashiCorp Boundary", "Access control for AI agents.", false},
		{"distinctive word", "Tracing LLM agents end to end", "Observability for agent pipelines with OpenTelemetry.", true},
		{"hyphenated product", "Distributed tracing with AWS X-Ray", "", true},
		{"spaceless product", "Xray sampling rules that save money", "", true},
		{"ray is not x-ray", "Scaling training jobs on Ray", "", false},
		{"acronym alias", "Error budgets for agent platforms", "A site reliability view of agent rollouts.", true},
		{"single product", "Long-term Prometheus storage with Thanos", "", true},
	}
	for _, c := range cases {
		got := confirmFocus([]Topic{{Topic: c.topic, Context: c.context, InFocus: true}}, intFocus)[0].InFocus
		if got != c.want {
			t.Errorf("%s: InFocus = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestConfirmFocusLeavesUncheckableAndOffFlags(t *testing.T) {
	// A focus list of only generic words gives nothing to check against.
	if !confirmFocus([]Topic{{Topic: "Anything", InFocus: true}}, []string{"AI Agents"})[0].InFocus {
		t.Error("uncheckable focus demoted a topic")
	}
	// Never promotes.
	if confirmFocus([]Topic{{Topic: "Grafana dashboards", InFocus: false}}, intFocus)[0].InFocus {
		t.Error("confirmFocus promoted an off-focus topic")
	}
	// No focus, no change.
	if !confirmFocus([]Topic{{Topic: "x", InFocus: true}}, nil)[0].InFocus {
		t.Error("no focus demoted a topic")
	}
}

// With the stricter check, an on-subject topic listed second wins the one
// slot over an adjacent one the model listed first and also marked in focus.
func TestSelectPrefersConfirmedFocus(t *testing.T) {
	topics := []Topic{
		{Topic: "Securing AI Agents with Boundary", InFocus: true},
		{Topic: "Grafana dashboards for agent latency", InFocus: true},
	}
	got := selectByFocus(confirmFocus(topics, intFocus), 1, true)
	if len(got) != 1 || got[0].Topic != "Grafana dashboards for agent latency" {
		t.Fatalf("got %#v", got)
	}
}
