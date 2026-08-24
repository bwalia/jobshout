package platformtools

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jobshout/server/internal/tools"
)

func TestCatalogScore_PhraseVsTokens(t *testing.T) {
	blob := "image_generate generate an image, picture, drawing or illustration from a text prompt. insight"
	if catalogScore("generate an image of a tiger", blob) < 2 {
		t.Fatalf("expected image tokens to score, got %d", catalogScore("generate an image of a tiger", blob))
	}
	if catalogScore("draw a tiger", blob) < 1 {
		t.Fatal("draw should hit the description")
	}
	if catalogScore("unrelated pentest scan", blob) != 0 {
		t.Fatal("unrelated query must not match")
	}
}

func TestCatalogSearch_FindsImageGenerateForNaturalQuery(t *testing.T) {
	reg := NewRegistry()
	reg.Register(newTool(
		"image_generate",
		"Generate an image, picture, drawing or illustration from a text prompt.",
		"insight", "", false, false,
		tools.ObjectSchema(map[string]any{}),
		nilRun,
	))
	registerCatalog(reg)
	ctx := WithPermissions(WithIdentity(context.Background(), Identity{OrgID: uuid.New(), UserID: uuid.New()}), nil)
	tool, ok := reg.Get("catalog_search")
	if !ok {
		t.Fatal("catalog_search missing")
	}
	res, err := tool.Run(ctx, map[string]any{"query": "generate an image of a tiger"})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := res.Data.(map[string]any)
	found := false
	switch names := data["names"].(type) {
	case []string:
		for _, n := range names {
			if n == "image_generate" {
				found = true
			}
		}
	}
	if found {
		t.Fatal("image_generate is AlwaysLoad so catalog_search should omit it")
	}
}

func TestAlwaysLoadIncludesImageGenerate(t *testing.T) {
	if !inAlwaysLoad("image_generate") {
		t.Fatal("image_generate must be always-load so chat does not route pictures to agent_execute")
	}
}
