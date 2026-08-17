package blog

import (
	"context"
	"strings"
	"testing"
)

// The exact diagram muse-glimmer produced in the model benchmark, which failed
// to render in all three runs. `bpf()` inside a node label is what breaks it.
const benchmarkFlowchart = "```mermaid\n" + `flowchart TD
    A[Userspace Source C] --> B[clang/LLVM → eBPF Bytecode]
    B --> C[Loader calls bpf() syscall]
    C --> D[Kernel Verifier]
    D -->|Reject| E[Load Error]
    D -->|Accept| F[JIT Compiler → Native Code]
` + "```"

func TestSanitiseDiagramsQuotesLabelsAndKeepsTheDiagram(t *testing.T) {
	got, notes := sanitiseDiagrams("Before.\n\n" + benchmarkFlowchart + "\n\nAfter.")

	if !strings.Contains(got, "```mermaid") {
		t.Fatalf("a repairable diagram was dropped:\n%s", got)
	}
	if !strings.Contains(got, `C["Loader calls bpf() syscall"]`) {
		t.Errorf("the label with parentheses was not quoted:\n%s", got)
	}
	if !strings.Contains(got, `A["Userspace Source C"]`) {
		t.Errorf("a plain label was left unquoted:\n%s", got)
	}
	// The edge labels between the pipes are not node labels and must survive.
	if !strings.Contains(got, `-->|Reject|`) {
		t.Errorf("an edge label was mangled:\n%s", got)
	}
	if len(notes) != 1 {
		t.Errorf("expected one note about the repair, got %v", notes)
	}
	if !strings.Contains(got, "Before.") || !strings.Contains(got, "After.") {
		t.Errorf("surrounding prose was lost:\n%s", got)
	}
}

// A state diagram's [*] is its start and end marker, not a node label. Quoting
// it would break a diagram that was already correct.
func TestSanitiseDiagramsLeavesStateDiagramsAlone(t *testing.T) {
	src := "```mermaid\n" + `stateDiagram-v2
    [*] --> Load
    Load --> Verify
    Verify --> JIT : verified
    Verify --> Reject : fails
    Reject --> [*]
` + "```"

	got, notes := sanitiseDiagrams(src)

	if !strings.Contains(got, "[*] --> Load") {
		t.Errorf("the start marker was rewritten:\n%s", got)
	}
	if len(notes) != 0 {
		t.Errorf("a valid state diagram was reported as changed: %v", notes)
	}
}

func TestSanitiseDiagramsLeavesSequenceDiagramsAlone(t *testing.T) {
	src := "```mermaid\n" + `sequenceDiagram
    participant C as Client
    participant S as Server
    C->>S: request
    S-->>C: response
` + "```"

	got, notes := sanitiseDiagrams(src)

	if got != src {
		t.Errorf("a valid sequence diagram was altered:\ngot  %s\nwant %s", got, src)
	}
	if len(notes) != 0 {
		t.Errorf("unexpected notes: %v", notes)
	}
}

func TestSanitiseDiagramsDropsWhatCannotRender(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string // substring expected in the note
	}{
		{"classDiagram", "classDiagram\n    class Foo {\n      +bar()\n    }", "classDiagram"},
		{"unknown type", "mindmap\n  root((hi))", "not a diagram type"},
		{"header only", "flowchart TD", "no content"},
		{"empty", "   \n\n", "empty"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := "Intro.\n\n```mermaid\n" + tc.body + "\n```\n\nOutro."

			got, notes := sanitiseDiagrams(src)

			if strings.Contains(got, "```mermaid") {
				t.Errorf("an unrenderable diagram survived:\n%s", got)
			}
			if len(notes) != 1 || !strings.Contains(notes[0], tc.want) {
				t.Errorf("notes = %v, want one mentioning %q", notes, tc.want)
			}
			if !strings.Contains(got, "Intro.") || !strings.Contains(got, "Outro.") {
				t.Errorf("surrounding prose was lost:\n%s", got)
			}
			if strings.Contains(got, "\n\n\n") {
				t.Errorf("removing the diagram left a blank gap:\n%q", got)
			}
		})
	}
}

// Code blocks in other languages are not diagrams and must not be touched.
func TestSanitiseDiagramsIgnoresOtherFences(t *testing.T) {
	src := "```go\nfunc main() { fmt.Println(\"hi\") }\n```"

	got, notes := sanitiseDiagrams(src)

	if got != src {
		t.Errorf("a Go block was altered:\ngot  %s\nwant %s", got, src)
	}
	if len(notes) != 0 {
		t.Errorf("unexpected notes: %v", notes)
	}
}

func TestSanitiseDiagramsHandlesSeveralBlocks(t *testing.T) {
	src := benchmarkFlowchart + "\n\n" +
		"```mermaid\nclassDiagram\n    class A {\n      +b()\n    }\n```" + "\n\n" +
		"```mermaid\nsequenceDiagram\n    A->>B: hi\n```"

	got, notes := sanitiseDiagrams(src)

	if n := strings.Count(got, "```mermaid"); n != 2 {
		t.Errorf("expected 2 surviving diagrams, got %d:\n%s", n, got)
	}
	if len(notes) != 2 {
		t.Errorf("expected a note for the repair and the drop, got %v", notes)
	}
}

// A label containing a double quote would otherwise close the quote being
// added and produce something worse than the original.
func TestRepairDiagramNeutralisesQuotesInLabels(t *testing.T) {
	body := "flowchart LR\n    A[the \"fast\" path] --> B[done]\n"

	got, changed := repairDiagram(body)

	if !changed {
		t.Fatal("expected a repair")
	}
	if strings.Contains(got, `A["the "fast" path"]`) {
		t.Errorf("nested double quotes were left in place:\n%s", got)
	}
	if !strings.Contains(got, `A["the 'fast' path"]`) {
		t.Errorf("inner quotes were not neutralised:\n%s", got)
	}
}

// Running twice must not double-quote what the first pass already quoted.
func TestRepairDiagramIsIdempotent(t *testing.T) {
	body := "flowchart TD\n    A[bpf() syscall] --> B[ok]\n"

	once, _ := repairDiagram(body)
	twice, changed := repairDiagram(once)

	if changed {
		t.Errorf("second pass changed an already-repaired diagram:\n%s", twice)
	}
	if twice != once {
		t.Errorf("not idempotent:\nfirst  %s\nsecond %s", once, twice)
	}
}

// Wiring: a broken diagram in the draft must be repaired by the time the
// article comes out of the pipeline, not merely by a direct call to
// sanitiseDiagrams.
func TestGenerateRepairsDiagramsInThePipeline(t *testing.T) {
	body := "# Title\n\nIntro paragraph.\n\n" + benchmarkFlowchart + "\n\nOutro."
	r := newTestRunner(nil, writeScript("Title", body)...)

	arts, err := r.Generate(context.Background(), GenerateRequest{Briefs: briefsFor("t")}, nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	md := arts[0].Markdown
	if !strings.Contains(md, `C["Loader calls bpf() syscall"]`) {
		t.Errorf("the pipeline did not repair the diagram:\n%s", md)
	}
}

// Wiring: an unrenderable diagram must not reach the stored article.
func TestGenerateDropsUnrenderableDiagramsInThePipeline(t *testing.T) {
	body := "# Title\n\nIntro paragraph.\n\n```mermaid\nclassDiagram\n    class Foo {\n      +bar()\n    }\n```\n\nOutro."
	r := newTestRunner(nil, writeScript("Title", body)...)

	arts, err := r.Generate(context.Background(), GenerateRequest{Briefs: briefsFor("t")}, nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	md := arts[0].Markdown
	if strings.Contains(md, "classDiagram") {
		t.Errorf("an unrenderable diagram survived the pipeline:\n%s", md)
	}
	if !strings.Contains(md, "Intro paragraph.") || !strings.Contains(md, "Outro.") {
		t.Errorf("surrounding prose was lost:\n%s", md)
	}
}
