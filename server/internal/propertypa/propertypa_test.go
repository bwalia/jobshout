package propertypa

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/model"
)

// promptHashes pins each released prompt version to its text. Changing a
// prompt without bumping its version fails here: OpsAPI audits runs by prompt
// version, so one version must always mean one text.
var promptHashes = map[string]string{
	"seller-pa@1.0.0":  "7441de042d5c08335cc5b0fac0ae955f0170d02a8b67c419ae5ae8a595bcb97a",
	"manager-pa@1.0.0": "2cb1791ca5164000df21fd17df25cb93fe7b1c7ea9d3079ca54553b4204559d6",
}

func TestPromptVersionsTrackPromptText(t *testing.T) {
	for _, d := range Definitions() {
		want, ok := promptHashes[d.PromptVersion]
		if !ok {
			t.Errorf("%s: new prompt version; add %q: %q to promptHashes", d.Key, d.PromptVersion, d.PromptSHA256())
			continue
		}
		if got := d.PromptSHA256(); got != want {
			t.Errorf("%s: prompt text changed but version %s did not; bump the version and pin %s", d.Key, d.PromptVersion, got)
		}
	}
}

func TestDefinitionsCarryTheRules(t *testing.T) {
	defs := Definitions()
	if len(defs) != 2 {
		t.Fatalf("want 2 agents, got %d", len(defs))
	}
	slugs := map[string]bool{}
	for _, s := range Skills() {
		slugs[s.Slug] = true
	}
	for _, d := range defs {
		for _, must := range []string{"Grounded or silent", "inference", "British English", OutputFenceTag, OutputContract,
			"never make or accept offers", "opsapi_get"} {
			if !strings.Contains(d.SystemPrompt, must) {
				t.Errorf("%s prompt lacks %q", d.Key, must)
			}
		}
		for _, tool := range []string{"opsapi_get", "opsapi_post"} {
			if !contains(d.Tools, tool) {
				t.Errorf("%s lacks tool %s", d.Key, tool)
			}
		}
		for _, tool := range d.Tools {
			if contains(ExternalApprovalTools, tool) && !contains(d.ApprovalTools, tool) {
				t.Errorf("%s: external tool %s is not approval-gated", d.Key, tool)
			}
			if tool == "http_request" || tool == "shell_command" {
				t.Errorf("%s: must not have %s", d.Key, tool)
			}
		}
		for _, s := range d.Skills {
			if !slugs[s] {
				t.Errorf("%s: unknown skill %s", d.Key, s)
			}
		}
		md := d.Metadata()
		if md[MetadataKey] != d.Key || md["prompt_version"] != d.PromptVersion || md["prompt_sha256"] != d.PromptSHA256() {
			t.Errorf("%s: metadata = %v", d.Key, md)
		}
	}
	if s, _ := DefinitionByKey(KeySeller); s.ContextRole != "seller_agent" {
		t.Errorf("seller context role = %q", s.ContextRole)
	}
	if m, _ := DefinitionByKey(KeyManager); m.ContextRole != "manager" {
		t.Errorf("manager context role = %q", m.ContextRole)
	}
	if !strings.Contains(mustPrompt("seller_pa.md"), "Questions come before money") {
		t.Error("seller prompt lost the coaching rule")
	}
}

// --- output contract ---

const goodOutput = "Sarah is your first call.\n\n- Second valuation was yesterday (event e_9).\n\n" +
	"```pa_output\n" +
	`{"contract":"property-pa.output.v1","mode":"Today","answer":"Sarah is your first call.",` +
	`"sources":[{"type":"Person","id":" p_1 ","label":"Sarah"},{"type":"event","id":"e_9"}],` +
	`"unknowns":["Tom's view"],"inferences":[],"actions":[{"kind":"task","id":"t_4"}],"needs_human":false}` +
	"\n```\n"

func TestParseOutput(t *testing.T) {
	out, prose, err := ParseOutput(goodOutput)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if out.Mode != "today" || len(out.Sources) != 2 || out.Sources[0].Type != "person" || out.Sources[0].ID != "p_1" {
		t.Fatalf("normalised output wrong: %+v", out)
	}
	if !strings.HasPrefix(prose, "Sarah is your first call.") || strings.Contains(prose, "pa_output") {
		t.Fatalf("prose = %q", prose)
	}
	if out.Actions[0].Kind != "task" || out.Unknowns[0] != "Tom's view" {
		t.Fatalf("output = %+v", out)
	}
}

func TestParseOutputAcceptsJSONTaggedBlockWithContract(t *testing.T) {
	text := "Answer.\n```json\n{\"other\":1}\n```\n```json\n" +
		`{"contract":"property-pa.output.v1","answer":"Answer.","sources":[]}` + "\n```"
	out, _, err := ParseOutput(text)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if out.Sources == nil || out.Actions == nil || out.Unknowns == nil {
		t.Fatal("nil arrays not normalised")
	}
}

func TestParseOutputRejects(t *testing.T) {
	if _, _, err := ParseOutput("Just prose, no block."); !errors.Is(err, ErrNoOutputBlock) {
		t.Fatalf("want ErrNoOutputBlock, got %v", err)
	}
	if _, _, err := ParseOutput("x\n```json\n{\"answer\":\"no contract\"}\n```"); !errors.Is(err, ErrNoOutputBlock) {
		t.Fatalf("a json block without the contract must not count: %v", err)
	}
	if _, _, err := ParseOutput("x\n```pa_output\n{not json}\n```"); err == nil {
		t.Fatal("invalid JSON accepted")
	}
	bad := "x\n```pa_output\n" + `{"contract":"property-pa.output.v1","answer":"","sources":[{"type":"rumour","id":""}],` +
		`"actions":[{"kind":"offer_made","id":"o1"}],"mode":"sell"}` + "\n```"
	out, _, err := ParseOutput(bad)
	if err == nil || out == nil {
		t.Fatal("invalid contract accepted")
	}
	for _, want := range []string{"answer is empty", `unknown type "rumour"`, "id is empty", `unknown kind "offer_made"`, `unknown mode "sell"`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q lacks %q", err, want)
		}
	}
}

// --- installer ---

type fakeAgents struct {
	byKey   map[string]*model.Agent
	names   map[string]bool
	created int
	updated int
}

func (f *fakeAgents) FindByPAKey(_ context.Context, _ uuid.UUID, key string) (*model.Agent, error) {
	return f.byKey[key], nil
}
func (f *fakeAgents) NameTaken(_ context.Context, _ uuid.UUID, name string) (bool, error) {
	return f.names[strings.ToLower(name)], nil
}
func (f *fakeAgents) Create(_ context.Context, a *model.Agent) error {
	f.created++
	f.byKey[a.Metadata[MetadataKey].(string)] = a
	f.names[strings.ToLower(a.Name)] = true
	return nil
}
func (f *fakeAgents) UpdateDefinition(_ context.Context, _, agentID uuid.UUID, role, desc, prompt string, md map[string]any) error {
	f.updated++
	for _, a := range f.byKey {
		if a.ID == agentID {
			a.Role, a.Description, a.SystemPrompt, a.EngineType = role, &desc, &prompt, model.EngineGoNative
			for k, v := range md {
				a.Metadata[k] = v
			}
		}
	}
	return nil
}

type fakeTools struct{ set map[uuid.UUID][]string }

func (f *fakeTools) Set(_ context.Context, id uuid.UUID, names []string) error {
	f.set[id] = names
	return nil
}

type fakeRules struct{ rules map[uuid.UUID][]string }

func (f *fakeRules) ListRules(_ context.Context, id uuid.UUID) ([]string, error) {
	return f.rules[id], nil
}
func (f *fakeRules) SetRules(_ context.Context, id uuid.UUID, names []string) error {
	f.rules[id] = names
	return nil
}

type fakeSkills struct {
	list             []model.Skill
	created, updated int
}

func (f *fakeSkills) List(context.Context, uuid.UUID) ([]model.Skill, error) { return f.list, nil }
func (f *fakeSkills) Create(_ context.Context, s *model.Skill) error {
	f.created++
	f.list = append(f.list, *s)
	return nil
}
func (f *fakeSkills) Update(_ context.Context, s *model.Skill) error {
	f.updated++
	for i := range f.list {
		if f.list[i].ID == s.ID {
			f.list[i] = *s
		}
	}
	return nil
}

func TestInstallIsIdempotentAndUpgrades(t *testing.T) {
	org, user := uuid.New(), uuid.New()
	agents := &fakeAgents{byKey: map[string]*model.Agent{}, names: map[string]bool{"seller pa": true}}
	tl := &fakeTools{set: map[uuid.UUID][]string{}}
	rules := &fakeRules{rules: map[uuid.UUID][]string{}}
	skills := &fakeSkills{list: []model.Skill{{ID: uuid.New(), Slug: SkillToday, OrgID: nil}}} // a builtin with the same slug is not ours
	in := &Installer{Agents: agents, Tools: tl, Approvals: rules, Skills: skills,
		HasTool: func(n string) bool { return n != "knowledge_search" }}

	res, err := in.Install(context.Background(), org, user)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if agents.created != 2 || len(res.Agents) != 2 || skills.created != len(Skills()) {
		t.Fatalf("created %d agents, %d skills: %+v", agents.created, skills.created, res)
	}
	seller := res.Agents[0]
	if seller.Key != KeySeller || seller.Status != StatusCreated || seller.Name != "Seller PA (Property PA)" {
		t.Fatalf("seller = %+v", seller)
	}
	if contains(seller.Tools, "knowledge_search") || !contains(seller.SkippedTools, "knowledge_search") {
		t.Fatalf("unregistered tool not skipped: %+v", seller)
	}
	sellerID := uuid.MustParse(seller.AgentID)
	if got := tl.set[sellerID]; !contains(got, "opsapi_get") || !contains(got, "opsapi_post") {
		t.Fatalf("tools = %v", got)
	}
	for _, g := range ExternalApprovalTools {
		if !contains(rules.rules[sellerID], g) {
			t.Fatalf("%s not gated: %v", g, rules.rules[sellerID])
		}
	}
	a := agents.byKey[KeySeller]
	if a.Metadata["prompt_version"] != SellerPromptVersion || a.ModelProvider != nil || a.EngineType != model.EngineGoNative {
		t.Fatalf("agent row = %+v", a)
	}

	// An admin added a gate; a re-install keeps it and changes nothing else.
	rules.rules[sellerID] = append(rules.rules[sellerID], "opsapi_post")
	res, err = in.Install(context.Background(), org, user)
	if err != nil {
		t.Fatal(err)
	}
	if agents.created != 2 || agents.updated != 0 || res.Agents[0].Status != StatusUnchanged || skills.updated != 0 {
		t.Fatalf("re-install not idempotent: created=%d updated=%d %+v", agents.created, agents.updated, res.Agents[0])
	}
	if !contains(rules.rules[sellerID], "opsapi_post") {
		t.Fatal("re-install dropped an admin's approval rule")
	}

	// An older prompt version is upgraded in place, keeping the admin's name.
	a.Name = "Sophie's PA"
	a.Metadata["prompt_version"] = "seller-pa@0.9.0"
	res, err = in.Install(context.Background(), org, user)
	if err != nil {
		t.Fatal(err)
	}
	if res.Agents[0].Status != StatusUpdated || agents.updated != 1 || a.Metadata["prompt_version"] != SellerPromptVersion ||
		res.Agents[0].Name != "Sophie's PA" {
		t.Fatalf("upgrade = %+v", res.Agents[0])
	}

	st, err := in.List(context.Background(), org)
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(st, func(i, j int) bool { return st[i].Key > st[j].Key })
	if !st[0].Installed || st[0].UpgradeAvailable || st[0].InstalledVersion != SellerPromptVersion {
		t.Fatalf("status = %+v", st)
	}
}
