package abtest

import "fmt"

func demoExperiments(host string) []Experiment {
	return []Experiment{{
		ID:        "abtesting",
		Name:      "AB Testing demo",
		Host:      host,
		RuleID:    "demo-abtesting-rule",
		ProfileID: "prod",
		Mode:      "demo",
		Backends: []Backend{
			{Label: "v1", Weight: 80, Role: "stable", Address: "origin-v1:8080"},
			{Label: "v2", Weight: 20, Role: "canary", Address: "origin-v2:8080"},
		},
		ObservePath: "/version",
		PublicURL:   "https://" + host,
		Note:        "Demo fixtures mirror abtesting.fictionally.org (80/20). Configure wslproxy to manage a real rule.",
		Writable:    true,
		WritePath:   PathDemo,
	}}
}

func demoObserve(ex *Experiment, n int) map[string]any {
	samples := make([]ObserveSample, 0, n)
	counts := map[string]int{"v1": 0, "v2": 0}
	// Deterministic pseudo-split ~80/20 for demos.
	for i := 0; i < n; i++ {
		v := "v1"
		if i%5 == 0 {
			v = "v2"
		}
		counts[v]++
		samples = append(samples, ObserveSample{Variant: v, Status: 200, LatencyMS: 12 + i%7})
	}
	expected := map[string]float64{}
	for _, b := range ex.Backends {
		expected[b.Label] = b.Weight
	}
	return map[string]any{
		"mode":       "demo",
		"experiment": ex.ID,
		"url":        ex.PublicURL + ex.ObservePath,
		"n":          n,
		"counts":     counts,
		"expected":   expected,
		"samples":    samples,
		"variants":   []string{"v1", "v2"},
		"message":    fmt.Sprintf("Demo observe (%d synthetic samples). Configure wslproxy to observe the real host.", n),
	}
}
