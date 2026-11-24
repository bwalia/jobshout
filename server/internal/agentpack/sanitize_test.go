package agentpack

import "testing"

func TestSanitizeKeepsWriterPostSettings(t *testing.T) {
	out := SanitizeEngineConfig(map[string]any{
		"writer_rules":      "No advice.",
		"writer_author":     "Portfolio Lab",
		"writer_categories": []any{"Market insights"},
		"writer_tags":       []any{"UK property"},
		"writer_cms_images": true,
	})
	for _, k := range []string{"writer_rules", "writer_author", "writer_categories", "writer_tags", "writer_cms_images"} {
		if _, ok := out[k]; !ok {
			t.Errorf("%s was dropped from the pack", k)
		}
	}
	if out["writer_cms_images"] != true {
		t.Errorf("writer_cms_images = %#v, want true", out["writer_cms_images"])
	}
}
