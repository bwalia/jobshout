package blog

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// liveCoverCases are real Workstation posts: title, the H2s the writer
// produced, and the layout their subject should draw. They double as the
// preview set (COVER_PREVIEW_DIR=/some/dir go test -run TestDiagramCover).
var liveCoverCases = []struct {
	slug, title, excerpt string
	headings             []string
	layout               coverLayout
}{
	{"cutting-ai-inference-costs-by-68", "Cutting AI Inference Costs by 68%", "AI inference costs are the compute and token charges…",
		[]string{"Caching: The Hidden Cost-Saver", "Batch Processing: Scaling Efficiency", "Reducing Context Windows: A Simple Optimization", "Upgrading GPUs: A Faster, Cheaper Path", "Real-World Examples: Banking, Education, and Beyond", "References"}, layoutBars},
	{"blast-radius-analytics", "Blast Radius Analytics", "Modern banking, healthtech, and tax platforms are no longer monoliths.",
		[]string{"The Complexity of Modern Distributed Microservices", "The Challenges of Root Cause Analysis", "Semantic Observability: A New Approach"}, layoutRipple},
	{"llm-drift-detection-in-regulated-environments", "LLM Drift Detection in Regulated Environments", "Regulated industries do not have the luxury of treating an LLM deployment as an experiment.",
		[]string{"The Importance of Responsible AI in Regulated Settings", "Technical Requirements for LLM Deployments", "Building a Dashboard for LLM Drift Detection", "Reducing Assessment Time with VERA"}, layoutThreshold},
	{"balancing-pruning-and-diversity", "Balancing Pruning and Diversity", "Generating many candidate reasoning paths and then selecting the best ones…",
		[]string{"What is Direct Pass@$k$ Optimization?", "The Trade-Off Between Quality and Diversity", "Applying Direct Pass@$k$ Optimization to Thought Pruning"}, layoutTree},
	{"generative-ai-s-hidden-impact", "Generative AI's Hidden Impact", "Visible gains in speed sit alongside less obvious shifts in accessibility…",
		[]string{"What is Generative AI and How Does it Work?", "Applications and Impact in Banking", "Tool and deployment trade-offs", "Accessibility Barriers with Generative AI"}, layoutIceberg},
	{"ai-s-double-edged-impact-on-devops", "AI's Double-Edged Impact on DevOps", "The same tooling can accelerate delivery in one workflow and degrade stability in another.",
		[]string{"The Unintended Consequences of AI-Generated Code", "How AI-Generated Tasks Can Boost DevOps Performance"}, layoutVersus},
	{"gateway-api-shifts-cluster-networking", "Gateway API Shifts Cluster Networking", "Cluster networking is moving from kube-proxy toward a Gateway API-first datapath.",
		[]string{"The End of kube-proxy", "Networking: How the Gateway API Changes Traffic Management", "Security: The Impact on Cluster Security", "Scalability: Multi-Cluster Management", "Migrating to a kube-proxy-free Setup"}, layoutHub},
	{"agentic-ai-in-production", "Agentic AI in Production", "Agentic AI shifts development from explicit control flow to goals, guardrails and observability.",
		[]string{"Agentic AI in Banking: Seamless Integration", "Operational Patterns for Agentic AI in Production"}, layoutLoop},
	{"aligning-workload-resilience-with-aws-resilience-hub", "Aligning Workload Resilience with AWS Resilience Hub", "Resilience requirements must be real, auditable and consistent across teams.",
		[]string{"Introducing AWS Resilience Hub: A Solution for Workload Resilience", "Labeling Support for Service Input Sources", "Generative AI-Powered Dependency Insights", "Resilience Policy Sharing through AWS Organizations"}, layoutHierarchy},
	{"pixel-space-diffusion-models-gain-ground", "Pixel-Space Diffusion Models Gain Ground", "Researchers are revisiting direct pixel generation.",
		[]string{"Rethinking the Design Space of Pixel-Space Diffusion Models", "Implications for Generative AI Models", "Trade-Offs in Speed, Interpretability, and Resource Requirements", "State-of-the-Art Models: HDiT and Sana"}, layoutFlow},
}

func markdownWith(headings []string) string {
	var b strings.Builder
	for _, h := range headings {
		b.WriteString("## " + h + "\n\nBody text.\n\n")
	}
	return b.String()
}

func TestDeriveCoverScene_PicksTheSubjectsLayout(t *testing.T) {
	for _, c := range liveCoverCases {
		s := deriveCoverScene(c.title, "", "", c.excerpt, markdownWith(c.headings))
		if s.Layout != c.layout {
			t.Errorf("%s: layout %q, want %q", c.slug, s.Layout, c.layout)
		}
	}
	s := deriveCoverScene("Cutting AI Inference Costs by 68%", "", "", "", "")
	if s.Stat != "−68%" {
		t.Errorf("stat from title = %q, want −68%%", s.Stat)
	}
}

func TestHeadingLabels_ShortensAndSkipsNonArgument(t *testing.T) {
	got := headingLabels(markdownWith(liveCoverCases[0].headings))
	want := []string{"Caching", "Batch Processing", "Reducing Context Windows", "Upgrading GPUs"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("labels = %q, want %q", got, want)
	}
	for _, l := range headingLabels(markdownWith(liveCoverCases[4].headings)) {
		if len(l) > maxCoverLabel {
			t.Errorf("label %q is longer than %d", l, maxCoverLabel)
		}
	}
}

func TestCoverScene_DecodesLooseShapes(t *testing.T) {
	cases := map[string]coverScene{
		`{"layout":"bars","labels":["caching 76","batching"],"stat":"-68%","eyebrow":"AI inference"}`: {Layout: layoutBars, Labels: []string{"caching 76", "batching"}, Stat: "-68%", Eyebrow: "AI inference"},
		`"pipeline"`: {Layout: layoutFlow},
		`{"layout":["hub"],"labels":"MCP, git, CI"}`:               {Layout: layoutHub, Labels: []string{"MCP", "git", "CI"}},
		`{"layout":"tree","labels":[{"name":"pass@k"}],"stat":60}`: {Layout: layoutTree, Labels: []string{"pass@k"}, Stat: "60"},
		`{"layout":"mosaic"}`:                                      {},
	}
	for in, want := range cases {
		var got coverScene
		if err := json.Unmarshal([]byte(in), &got); err != nil {
			t.Fatalf("%s: %v", in, err)
		}
		if got.Layout != want.Layout || strings.Join(got.Labels, "|") != strings.Join(want.Labels, "|") || got.Stat != want.Stat || got.Eyebrow != want.Eyebrow {
			t.Errorf("%s: got %+v, want %+v", in, got, want)
		}
	}
	// Inside a plan, a broken scene must not fail the plan.
	var p writePlan
	if err := json.Unmarshal([]byte(`{"title":"T","cover_scene":[1,2]}`), &p); err != nil || p.Title != "T" {
		t.Fatalf("plan with an odd cover_scene: %v %+v", err, p)
	}
}

func TestDiagramCover_EveryLayoutRenders(t *testing.T) {
	dir := os.Getenv("COVER_PREVIEW_DIR")
	for _, c := range liveCoverCases {
		a := &GeneratedArticle{Title: c.title, Slug: c.slug, Excerpt: c.excerpt, Markdown: markdownWith(c.headings)}
		scene := coverSceneFor(a)
		b, err := drawDiagramCover(scene, accentForTopic(c.title, "", ""), c.slug)
		if err != nil {
			t.Fatalf("%s: %v", c.slug, err)
		}
		img, err := png.Decode(bytes.NewReader(b))
		if err != nil {
			t.Fatalf("%s: not a PNG: %v", c.slug, err)
		}
		if bnd := img.Bounds(); bnd.Dx() != int(designW) || bnd.Dy() != int(designH) {
			t.Fatalf("%s: %v, want %vx%v", c.slug, bnd, designW, designH)
		}
		if dir != "" {
			if err := os.WriteFile(filepath.Join(dir, c.slug+".png"), b, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	// A scene with no labels still draws every layout from its defaults.
	for _, l := range coverLayouts {
		if _, err := drawDiagramCover(coverScene{Layout: l}, "", "seed"); err != nil {
			t.Fatalf("%s with no labels: %v", l, err)
		}
	}
}

// The default style draws a diagram cover with no image service at all, so
// every post gets a cover about its subject.
func TestEnsureCover_DrawsDiagramWithoutImageService(t *testing.T) {
	r := newTestRunner(&fakeCMS{})
	a := &GeneratedArticle{Title: "Blast Radius Analytics", Slug: "blast-radius-analytics", Markdown: markdownWith(liveCoverCases[1].headings)}
	url, err := r.ensureCover(context.Background(), uuid.New(), a, nil)
	if err != nil {
		t.Fatal(err)
	}
	if a.CoverImageProvider != "diagram" || a.CoverImageModel != "scene:ripple" || !strings.HasPrefix(url, "https://jobshout.test/api/v1/images/file/") {
		t.Fatalf("provider=%q model=%q url=%q", a.CoverImageProvider, a.CoverImageModel, url)
	}
	if !strings.Contains(a.CoverImagePrompt, `"layout":"ripple"`) {
		t.Fatalf("scene not recorded: %q", a.CoverImagePrompt)
	}
}

// A picture under the diagram shows through it: the cover is the art, dimmed,
// with the scene lettered on top. COVER_ART names a picture to preview with.
func TestDrawSceneOn_ArtShowsThrough(t *testing.T) {
	var art image.Image
	if f := os.Getenv("COVER_ART"); f != "" {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if art, _, err = image.Decode(bytes.NewReader(b)); err != nil {
			t.Fatal(err)
		}
	} else {
		art = image.NewRGBA(image.Rect(0, 0, 1280, 720))
		draw.Draw(art.(*image.RGBA), art.Bounds(), image.NewUniform(color.RGBA{0xf0, 0x80, 0x20, 0xff}), image.Point{}, draw.Src)
	}
	scene := coverScene{Layout: layoutFlow, Labels: []string{"HTTP request", "auth", "rate limit", "routing policy", "proxy to pod"}, Eyebrow: "API SECURITY"}
	over, err := drawSceneOn(art, scene, "amber", "seed")
	if err != nil {
		t.Fatal(err)
	}
	plain, _ := drawDiagramCover(scene, "amber", "seed")
	if dir := os.Getenv("COVER_PREVIEW_DIR"); dir != "" {
		_ = os.WriteFile(filepath.Join(dir, "art-flow.png"), over, 0o644)
	}
	a, _ := png.Decode(bytes.NewReader(over))
	g, _ := png.Decode(bytes.NewReader(plain))
	if bnd := a.Bounds(); bnd.Dx() != int(designW) || bnd.Dy() != int(designH) {
		t.Fatalf("%v, want %vx%v", bnd, designW, designH)
	}
	// A corner, outside the diagram, is the art dimmed: warmer than the ground.
	ar, _, _, _ := a.At(20, 20).RGBA()
	gr, _, _, _ := g.At(20, 20).RGBA()
	if ar <= gr {
		t.Fatalf("corner red %d over art, %d on the ground: the art does not show", ar>>8, gr>>8)
	}
}
