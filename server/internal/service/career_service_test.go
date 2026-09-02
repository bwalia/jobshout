package service

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/career"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
	"github.com/jobshout/server/internal/research"
)

type careerMem struct {
	mu         sync.Mutex
	profiles   map[string]*model.CareerProfile // org:user
	blacklist  map[uuid.UUID][]model.CareerBlacklistEntry
	pipeline   map[uuid.UUID][]model.CareerPipelineItem
	apps       map[uuid.UUID]*model.CareerApplication
	evals      map[uuid.UUID]*model.CareerEvaluation
	artifacts  []model.CareerArtifact
	stories    []model.CareerStory
	portals    []model.CareerPortal
	scanEvents map[string]bool
	scanRuns   []*model.CareerScanRun
	events     []model.CareerStatusEvent
	followups  []model.CareerFollowup
	contacts   []model.CareerContact
	offers     []model.CareerOffer
	salaries   []model.CareerSalaryObservation
	documents  []model.CareerDocument
}

func newCareerMem() *careerMem {
	return &careerMem{
		profiles:   map[string]*model.CareerProfile{},
		blacklist:  map[uuid.UUID][]model.CareerBlacklistEntry{},
		pipeline:   map[uuid.UUID][]model.CareerPipelineItem{},
		apps:       map[uuid.UUID]*model.CareerApplication{},
		evals:      map[uuid.UUID]*model.CareerEvaluation{},
		scanEvents: map[string]bool{},
	}
}

func profileKey(org, user uuid.UUID) string { return org.String() + ":" + user.String() }

func (m *careerMem) UpsertProfile(_ context.Context, p *model.CareerProfile) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	p.UpdatedAt = time.Now()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = p.UpdatedAt
	}
	cp := *p
	m.profiles[profileKey(p.OrgID, p.UserID)] = &cp
	return nil
}
func (m *careerMem) GetProfileByUser(_ context.Context, orgID, userID uuid.UUID) (*model.CareerProfile, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.profiles[profileKey(orgID, userID)]
	if p == nil {
		return nil, nil
	}
	cp := *p
	return &cp, nil
}
func (m *careerMem) GetProfileByID(_ context.Context, orgID, id uuid.UUID) (*model.CareerProfile, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, p := range m.profiles {
		if p.OrgID == orgID && p.ID == id {
			cp := *p
			return &cp, nil
		}
	}
	return nil, nil
}
func (m *careerMem) InsertProfileVersion(context.Context, uuid.UUID, uuid.UUID, string, string) error {
	return nil
}
func (m *careerMem) InsertDocument(_ context.Context, d *model.CareerDocument) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if d.ID == uuid.Nil {
		d.ID = uuid.New()
	}
	d.CreatedAt = time.Now()
	m.documents = append(m.documents, *d)
	return nil
}
func (m *careerMem) ListBlacklist(_ context.Context, _, profileID uuid.UUID) ([]model.CareerBlacklistEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := m.blacklist[profileID]
	if out == nil {
		return []model.CareerBlacklistEntry{}, nil
	}
	return append([]model.CareerBlacklistEntry{}, out...), nil
}
func (m *careerMem) InsertBlacklist(_ context.Context, e *model.CareerBlacklistEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e.ID == uuid.Nil {
		e.ID = uuid.New()
	}
	e.CreatedAt = time.Now()
	m.blacklist[e.ProfileID] = append(m.blacklist[e.ProfileID], *e)
	return nil
}
func (m *careerMem) ListPortals(_ context.Context, _, profileID uuid.UUID) ([]model.CareerPortal, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []model.CareerPortal
	for _, p := range m.portals {
		if p.ProfileID == profileID {
			out = append(out, p)
		}
	}
	if out == nil {
		out = []model.CareerPortal{}
	}
	return out, nil
}
func (m *careerMem) UpsertPortal(_ context.Context, p *model.CareerPortal) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	p.UpdatedAt = time.Now()
	key := p.Board + ":" + strings.ToLower(p.Slug)
	for i, existing := range m.portals {
		if existing.ProfileID == p.ProfileID && existing.Board+":"+strings.ToLower(existing.Slug) == key {
			m.portals[i] = *p
			return nil
		}
	}
	m.portals = append(m.portals, *p)
	return nil
}
func (m *careerMem) UpsertPipelineItem(_ context.Context, it *model.CareerPipelineItem) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if it.ID == uuid.Nil {
		it.ID = uuid.New()
	}
	it.UpdatedAt = time.Now()
	if it.CreatedAt.IsZero() {
		it.CreatedAt = it.UpdatedAt
	}
	list := m.pipeline[it.ProfileID]
	for i, existing := range list {
		if existing.ListingURL == it.ListingURL {
			it.ID = existing.ID
			list[i] = *it
			m.pipeline[it.ProfileID] = list
			return nil
		}
	}
	m.pipeline[it.ProfileID] = append(list, *it)
	return nil
}
func (m *careerMem) GetPipelineByURL(_ context.Context, profileID uuid.UUID, listingURL string) (*model.CareerPipelineItem, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, it := range m.pipeline[profileID] {
		if it.ListingURL == listingURL {
			cp := it
			return &cp, nil
		}
	}
	return nil, nil
}
func (m *careerMem) ListPipeline(_ context.Context, _, profileID uuid.UUID, pagination model.PaginationParams) (*model.PaginatedResponse[model.CareerPipelineItem], error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	data := m.pipeline[profileID]
	if data == nil {
		data = []model.CareerPipelineItem{}
	}
	pagination.Normalize()
	return &model.PaginatedResponse[model.CareerPipelineItem]{Data: data, Total: len(data), Page: pagination.Page, PerPage: pagination.PerPage}, nil
}
func (m *careerMem) UpdatePipeline(_ context.Context, it *model.CareerPipelineItem) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	list := m.pipeline[it.ProfileID]
	for i := range list {
		if list[i].ID == it.ID {
			list[i] = *it
		}
	}
	return nil
}
func (m *careerMem) UpsertApplication(_ context.Context, a *model.CareerApplication) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	a.UpdatedAt = time.Now()
	if a.CreatedAt.IsZero() {
		a.CreatedAt = a.UpdatedAt
	}
	if a.ListingURL != "" {
		for _, existing := range m.apps {
			if existing.ProfileID == a.ProfileID && existing.ListingURL == a.ListingURL {
				a.ID = existing.ID
				break
			}
		}
	}
	cp := *a
	m.apps[a.ID] = &cp
	return nil
}
func (m *careerMem) GetApplication(_ context.Context, orgID, id uuid.UUID) (*model.CareerApplication, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	a := m.apps[id]
	if a == nil || a.OrgID != orgID {
		return nil, nil
	}
	cp := *a
	return &cp, nil
}
func (m *careerMem) GetApplicationByURL(_ context.Context, profileID uuid.UUID, listingURL string) (*model.CareerApplication, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if listingURL == "" {
		return nil, nil
	}
	for _, a := range m.apps {
		if a.ProfileID == profileID && a.ListingURL == listingURL {
			cp := *a
			return &cp, nil
		}
	}
	return nil, nil
}
func (m *careerMem) ListApplications(_ context.Context, orgID, profileID uuid.UUID, status string, pagination model.PaginationParams) (*model.PaginatedResponse[model.CareerApplication], error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var data []model.CareerApplication
	for _, a := range m.apps {
		if a.OrgID == orgID && a.ProfileID == profileID && (status == "" || a.Status == status) {
			data = append(data, *a)
		}
	}
	if data == nil {
		data = []model.CareerApplication{}
	}
	pagination.Normalize()
	return &model.PaginatedResponse[model.CareerApplication]{Data: data, Total: len(data), Page: 1, PerPage: pagination.PerPage}, nil
}
func (m *careerMem) UpdateApplication(_ context.Context, a *model.CareerApplication) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	a.UpdatedAt = time.Now()
	cp := *a
	m.apps[a.ID] = &cp
	return nil
}
func (m *careerMem) AppendStatusEvent(_ context.Context, e *model.CareerStatusEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	e.CreatedAt = time.Now()
	m.events = append(m.events, *e)
	return nil
}
func (m *careerMem) ListStatusEvents(_ context.Context, applicationID uuid.UUID) ([]model.CareerStatusEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []model.CareerStatusEvent
	for _, e := range m.events {
		if e.ApplicationID == applicationID {
			out = append(out, e)
		}
	}
	return out, nil
}
func (m *careerMem) InsertEvaluation(_ context.Context, e *model.CareerEvaluation) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e.ID == uuid.Nil {
		e.ID = uuid.New()
	}
	e.CreatedAt = time.Now()
	cp := *e
	m.evals[e.ID] = &cp
	return nil
}
func (m *careerMem) GetEvaluation(_ context.Context, orgID, id uuid.UUID) (*model.CareerEvaluation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.evals[id]
	if e == nil || e.OrgID != orgID {
		return nil, nil
	}
	cp := *e
	return &cp, nil
}
func (m *careerMem) ListEvaluations(_ context.Context, orgID, profileID uuid.UUID, pagination model.PaginationParams) (*model.PaginatedResponse[model.CareerEvaluation], error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var data []model.CareerEvaluation
	for _, e := range m.evals {
		if e.OrgID == orgID && e.ProfileID == profileID {
			data = append(data, *e)
		}
	}
	if data == nil {
		data = []model.CareerEvaluation{}
	}
	pagination.Normalize()
	return &model.PaginatedResponse[model.CareerEvaluation]{Data: data, Total: len(data), Page: 1, PerPage: pagination.PerPage}, nil
}
func (m *careerMem) InsertArtifact(_ context.Context, a *model.CareerArtifact) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	a.CreatedAt = time.Now()
	if len(a.FileBytes) > 0 {
		a.HasPDF = true
	}
	m.artifacts = append(m.artifacts, *a)
	return nil
}
func (m *careerMem) GetArtifact(_ context.Context, orgID, id uuid.UUID) (*model.CareerArtifact, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, a := range m.artifacts {
		if a.OrgID == orgID && a.ID == id {
			cp := a
			return &cp, nil
		}
	}
	return nil, nil
}
func (m *careerMem) ListArtifacts(_ context.Context, applicationID uuid.UUID) ([]model.CareerArtifact, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []model.CareerArtifact
	for _, a := range m.artifacts {
		if a.ApplicationID != nil && *a.ApplicationID == applicationID {
			out = append(out, a)
		}
	}
	return out, nil
}
func (m *careerMem) ListStories(_ context.Context, _, profileID uuid.UUID) ([]model.CareerStory, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []model.CareerStory
	for _, s := range m.stories {
		if s.ProfileID == profileID {
			out = append(out, s)
		}
	}
	if out == nil {
		out = []model.CareerStory{}
	}
	return out, nil
}
func (m *careerMem) GetStoryByID(_ context.Context, id uuid.UUID) (*model.CareerStory, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.stories {
		if m.stories[i].ID == id {
			s := m.stories[i]
			return &s, nil
		}
	}
	return nil, nil
}
func (m *careerMem) UpsertStory(_ context.Context, s *model.CareerStory) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	s.UpdatedAt = time.Now()
	if s.CreatedAt.IsZero() {
		s.CreatedAt = s.UpdatedAt
	}
	for i := range m.stories {
		if m.stories[i].ID == s.ID {
			m.stories[i] = *s
			return nil
		}
	}
	m.stories = append(m.stories, *s)
	return nil
}
func (m *careerMem) InsertScanRun(_ context.Context, r *model.CareerScanRun) error {
	if r.ID == uuid.Nil {
		r.ID = uuid.New()
	}
	r.CreatedAt = time.Now()
	m.scanRuns = append(m.scanRuns, r)
	return nil
}
func (m *careerMem) UpdateScanRun(_ context.Context, r *model.CareerScanRun) error { return nil }
func (m *careerMem) HasScanEvent(_ context.Context, profileID uuid.UUID, listingURL string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.scanEvents[profileID.String()+listingURL], nil
}
func (m *careerMem) InsertScanEvent(_ context.Context, _, profileID uuid.UUID, listingURL, _, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.scanEvents[profileID.String()+listingURL] = true
	return nil
}
func (m *careerMem) InsertFollowup(_ context.Context, f *model.CareerFollowup) error {
	if f.ID == uuid.Nil {
		f.ID = uuid.New()
	}
	f.CreatedAt = time.Now()
	m.followups = append(m.followups, *f)
	return nil
}
func (m *careerMem) ListFollowups(_ context.Context, _, profileID uuid.UUID) ([]model.CareerFollowup, error) {
	var out []model.CareerFollowup
	for _, f := range m.followups {
		if f.ProfileID == profileID {
			out = append(out, f)
		}
	}
	return out, nil
}

func (m *careerMem) GetLatestEvaluationForApp(_ context.Context, orgID, applicationID uuid.UUID) (*model.CareerEvaluation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var best *model.CareerEvaluation
	for _, e := range m.evals {
		if e.OrgID == orgID && e.ApplicationID != nil && *e.ApplicationID == applicationID {
			if best == nil || e.CreatedAt.After(best.CreatedAt) {
				cp := *e
				best = &cp
			}
		}
	}
	return best, nil
}

func (m *careerMem) InsertContact(_ context.Context, c *model.CareerContact) error {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	c.CreatedAt = time.Now()
	m.contacts = append(m.contacts, *c)
	return nil
}

func (m *careerMem) ListContacts(_ context.Context, _, profileID uuid.UUID) ([]model.CareerContact, error) {
	var out []model.CareerContact
	for _, c := range m.contacts {
		if c.ProfileID == profileID {
			out = append(out, c)
		}
	}
	if out == nil {
		out = []model.CareerContact{}
	}
	return out, nil
}

func (m *careerMem) InsertOffer(_ context.Context, o *model.CareerOffer) error {
	if o.ID == uuid.Nil {
		o.ID = uuid.New()
	}
	o.CreatedAt = time.Now()
	m.offers = append(m.offers, *o)
	return nil
}

func (m *careerMem) InsertSalaryObservation(_ context.Context, o *model.CareerSalaryObservation) error {
	if o.ID == uuid.Nil {
		o.ID = uuid.New()
	}
	o.CreatedAt = time.Now()
	m.salaries = append(m.salaries, *o)
	return nil
}

var _ repository.CareerRepository = (*careerMem)(nil)

type careerTestAgents struct {
	byBuiltin map[string]*model.Agent
}

func (a *careerTestAgents) Create(_ context.Context, agent *model.Agent) error {
	if agent.Metadata != nil {
		if b, ok := agent.Metadata[model.MetadataKeyBuiltin].(string); ok {
			cp := *agent
			a.byBuiltin[agent.OrgID.String()+":"+b] = &cp
		}
	}
	return nil
}
func (a *careerTestAgents) FindBuiltin(_ context.Context, orgID uuid.UUID, builtin string) (*model.Agent, error) {
	ag := a.byBuiltin[orgID.String()+":"+builtin]
	if ag == nil {
		return nil, nil
	}
	cp := *ag
	return &cp, nil
}
func (a *careerTestAgents) FindByID(context.Context, uuid.UUID) (*model.Agent, error) {
	return nil, nil
}
func (a *careerTestAgents) ListByOrg(context.Context, uuid.UUID, model.PaginationParams, repository.AgentListFilter) (*model.PaginatedResponse[model.Agent], error) {
	return nil, nil
}
func (a *careerTestAgents) Update(_ context.Context, agent *model.Agent) error {
	if agent.Metadata != nil {
		if b, ok := agent.Metadata[model.MetadataKeyBuiltin].(string); ok {
			cp := *agent
			a.byBuiltin[agent.OrgID.String()+":"+b] = &cp
		}
	}
	return nil
}
func (a *careerTestAgents) Delete(context.Context, uuid.UUID) error               { return nil }
func (a *careerTestAgents) UpdateStatus(context.Context, uuid.UUID, string) error { return nil }

func setupCareer(t *testing.T) (*careerService, *careerMem, uuid.UUID, uuid.UUID) {
	t.Helper()
	repo := newCareerMem()
	agents := &careerTestAgents{byBuiltin: map[string]*model.Agent{}}
	svc := NewCareerService(repo, agents, nil, nil, nil, zap.NewNop()).(*careerService)
	return svc, repo, uuid.New(), uuid.New()
}

func TestEnsureCareerOpsCreatesOnce(t *testing.T) {
	svc, _, orgID, _ := setupCareer(t)
	a1, err := svc.EnsureCareerOps(context.Background(), orgID)
	if err != nil {
		t.Fatal(err)
	}
	a2, err := svc.EnsureCareerOps(context.Background(), orgID)
	if err != nil {
		t.Fatal(err)
	}
	if a1.ID != a2.ID {
		t.Fatal("must seed once")
	}
	if !a1.IsBuiltin(model.BuiltinCareerOps) {
		t.Fatal("missing builtin marker")
	}
	if a1.Name != model.AgentNameCareerOps {
		t.Fatalf("name = %q", a1.Name)
	}
	if a1.Role != "Career Agent" {
		t.Fatalf("role = %q", a1.Role)
	}
}

func TestEnsureCareerOpsRenamesLegacy(t *testing.T) {
	repo := newCareerMem()
	agents := &careerTestAgents{byBuiltin: map[string]*model.Agent{}}
	orgID := uuid.New()
	legacy := careerOpsSeed(orgID)
	legacy.Name = "CareerOps"
	legacy.Role = "Career"
	if err := agents.Create(context.Background(), legacy); err != nil {
		t.Fatal(err)
	}
	svc := NewCareerService(repo, agents, nil, nil, nil, zap.NewNop()).(*careerService)
	got, err := svc.EnsureCareerOps(context.Background(), orgID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != legacy.ID {
		t.Fatal("must keep the existing agent")
	}
	if got.Name != model.AgentNameCareerOps || got.Role != "Career Agent" {
		t.Fatalf("got name=%q role=%q", got.Name, got.Role)
	}
}

func TestEvaluateTailorsBelowApplyFloor(t *testing.T) {
	svc, _, orgID, userID := setupCareer(t)
	_, _ = svc.UpdateProfile(context.Background(), orgID, userID, model.UpdateCareerProfileRequest{
		CVMarkdown: ptr("# Ada\n\n## Experience\n- baker\n"),
		Identity:   &model.CareerIdentity{FullName: "Ada"},
	})
	res, err := svc.Evaluate(context.Background(), orgID, userID, model.EvaluateCareerRequest{
		JDText:   career.GoldenJDForTest(),
		TailorCV: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Evaluation == nil {
		t.Fatal("expected evaluation")
	}
	if res.Evaluation.Score.RecommendApply {
		t.Fatal("weak CV should sit below the apply floor — tailor must still run")
	}
	found := false
	for _, a := range res.Artifacts {
		if a.Kind == model.CareerArtifactCV && a.BodyMarkdown != "" {
			found = true
			if !career.SameOutline("# Ada\n\n## Experience\n- baker\n", a.BodyMarkdown) &&
				!strings.Contains(a.BodyMarkdown, "layout unchanged") {
				t.Fatalf("tailored CV must keep outline or fall back: %s", a.BodyMarkdown)
			}
		}
	}
	if !found {
		t.Fatal("expected a tailored CV artifact below 4.0")
	}
}

func TestUploadCVSavesPDFAndRejectsOtherTypes(t *testing.T) {
	svc, repo, orgID, userID := setupCareer(t)
	_, err := svc.UploadCV(context.Background(), orgID, userID, "cv.md", "text/markdown", []byte("# Jane Doe\nStaff engineer"))
	if err == nil {
		t.Fatal("markdown upload must fail")
	}
	pdf := []byte("%PDF-1.1\n<< /Length 48 >>\nstream\nBT\n(Jane Doe) Tj\n(jane@example.com) Tj\n(Staff engineer) Tj\nET\nendstream\n")
	prop, err := svc.UploadCV(context.Background(), orgID, userID, "cv.pdf", "application/pdf", pdf)
	if err != nil {
		t.Fatal(err)
	}
	if prop.Patch.CVMarkdown == nil || !strings.Contains(*prop.Patch.CVMarkdown, "Jane") {
		t.Fatalf("expected extracted CV: %+v", prop)
	}
	p, _ := svc.GetOrCreateProfile(context.Background(), orgID, userID)
	if !strings.Contains(p.CVMarkdown, "Jane") {
		t.Fatal("PDF upload must save the profile CV")
	}
	if len(repo.documents) != 1 {
		t.Fatalf("expected stored document, got %d", len(repo.documents))
	}
}

func TestTailorCVAttachesPDF(t *testing.T) {
	svc, _, orgID, userID := setupCareer(t)
	_, _ = svc.UpdateProfile(context.Background(), orgID, userID, model.UpdateCareerProfileRequest{
		CVMarkdown: ptr("# Ada Lovelace\n\nStaff engineer. Kubernetes GPU scheduling."),
		Identity:   &model.CareerIdentity{FullName: "Ada Lovelace"},
	})
	res, err := svc.Evaluate(context.Background(), orgID, userID, model.EvaluateCareerRequest{JDText: career.GoldenJDForTest()})
	if err != nil || res.Evaluation == nil {
		t.Fatalf("evaluate: %v %+v", err, res)
	}
	art, err := svc.TailorCV(context.Background(), orgID, userID, res.Evaluation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !art.HasPDF || art.PDFBase64 == "" || !strings.HasSuffix(art.PDFFilename, ".pdf") {
		t.Fatalf("expected downloadable PDF: %+v", art)
	}
	got, pdf, err := svc.ArtifactPDF(context.Background(), orgID, userID, art.ID)
	if err != nil || got == nil || !bytes.HasPrefix(pdf, []byte("%PDF")) {
		t.Fatalf("artifact pdf: %v %d", err, len(pdf))
	}
}

func TestEvaluatePersistsTracker(t *testing.T) {
	svc, _, orgID, userID := setupCareer(t)
	_, _ = svc.UpdateProfile(context.Background(), orgID, userID, model.UpdateCareerProfileRequest{
		CVMarkdown: ptr("Staff engineer. Kubernetes GPU scheduling observability distributed systems."),
		Identity:   &model.CareerIdentity{FullName: "Ada"},
	})
	res, err := svc.Evaluate(context.Background(), orgID, userID, model.EvaluateCareerRequest{JDText: career.GoldenJDForTest()})
	if err != nil {
		t.Fatal(err)
	}
	if res.Evaluation == nil || res.Application == nil {
		t.Fatal("expected evaluation and application")
	}
	if res.Application.Status != model.CareerStatusEvaluated && res.Application.Status != model.CareerStatusSkip {
		t.Fatalf("status = %s", res.Application.Status)
	}
}

func TestBlacklistAsks(t *testing.T) {
	svc, _, orgID, userID := setupCareer(t)
	_, _ = svc.AddBlacklist(context.Background(), orgID, userID, model.AddCareerBlacklistRequest{Company: "Northwind Labs"})
	res, err := svc.Evaluate(context.Background(), orgID, userID, model.EvaluateCareerRequest{JDText: career.GoldenJDForTest()})
	if err != nil {
		t.Fatal(err)
	}
	if res.BlacklistHit == nil {
		t.Fatal("expected blacklist hit, not a silent skip")
	}
	if res.Evaluation != nil {
		t.Fatal("must not continue until confirmed")
	}
	res2, err := svc.Evaluate(context.Background(), orgID, userID, model.EvaluateCareerRequest{
		JDText: career.GoldenJDForTest(), ConfirmBlacklist: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res2.Evaluation == nil {
		t.Fatal("confirm should evaluate")
	}
}

func TestSetStatusMachine(t *testing.T) {
	svc, _, orgID, userID := setupCareer(t)
	res, err := svc.Evaluate(context.Background(), orgID, userID, model.EvaluateCareerRequest{JDText: career.GoldenJDForTest()})
	if err != nil {
		t.Fatal(err)
	}
	if res.Application.Status == model.CareerStatusSkip {
		// sponsorship hard-stop on empty profile — reopen via evaluated first
		return
	}
	_, err = svc.SetStatus(context.Background(), orgID, userID, res.Application.ID, model.CareerStatusHired, "")
	if err == nil {
		t.Fatal("evaluated → hired must fail")
	}
	a, err := svc.SetStatus(context.Background(), orgID, userID, res.Application.ID, model.CareerStatusApplied, "sent")
	if err != nil {
		t.Fatal(err)
	}
	if a.Status != model.CareerStatusApplied {
		t.Fatalf("status=%s", a.Status)
	}
}

func TestOrgIsolation(t *testing.T) {
	svc, _, orgA, userA := setupCareer(t)
	orgB, userB := uuid.New(), uuid.New()
	res, err := svc.Evaluate(context.Background(), orgA, userA, model.EvaluateCareerRequest{JDText: career.GoldenJDForTest()})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.GetEvaluation(context.Background(), orgB, userB, res.Evaluation.ID)
	if err != ErrCareerNotFound {
		t.Fatalf("other org must not read eval: %v", err)
	}
}

func TestDoctorOnEmpty(t *testing.T) {
	svc, _, orgID, userID := setupCareer(t)
	rep, err := svc.Doctor(context.Background(), orgID, userID)
	if err != nil {
		t.Fatal(err)
	}
	if rep.OK {
		t.Fatal("empty profile is not healthy")
	}
}

func TestIntakeDoesNotWrite(t *testing.T) {
	svc, _, orgID, userID := setupCareer(t)
	prop, err := svc.Intake(context.Background(), orgID, userID, "# Jane Doe\njane@example.com\nStaff engineer")
	if err != nil {
		t.Fatal(err)
	}
	if prop.Patch.Identity == nil || !strings.Contains(prop.Patch.Identity.Email, "jane") {
		t.Fatalf("expected email in proposal: %+v", prop)
	}
	p, _ := svc.GetOrCreateProfile(context.Background(), orgID, userID)
	if p.CVMarkdown != "" {
		t.Fatal("intake must not write until confirm")
	}
}

func TestFollowupSeededOnApplied(t *testing.T) {
	svc, repo, orgID, userID := setupCareer(t)
	res, err := svc.Evaluate(context.Background(), orgID, userID, model.EvaluateCareerRequest{JDText: career.GoldenJDForTest()})
	if err != nil {
		t.Fatal(err)
	}
	if res.Application.Status == model.CareerStatusSkip {
		t.Skip("hard-stop skip — no applied transition")
	}
	if _, err := svc.SetStatus(context.Background(), orgID, userID, res.Application.ID, model.CareerStatusApplied, "sent by hand"); err != nil {
		t.Fatal(err)
	}
	fus, err := svc.ListFollowups(context.Background(), orgID, userID)
	if err != nil {
		t.Fatal(err)
	}
	if len(fus) == 0 {
		t.Fatal("applied should seed a follow-up draft")
	}
	if !strings.Contains(strings.ToLower(fus[0].Draft), "not sent") {
		t.Fatalf("follow-up must be draft-only: %s", fus[0].Draft)
	}
	if repo == nil {
		t.Fatal("mem repo")
	}
}

func TestHighScorePersistsBlockHAndCover(t *testing.T) {
	svc, _, orgID, userID := setupCareer(t)
	jd := career.GoldenJDForTest()
	_, _ = svc.UpdateProfile(context.Background(), orgID, userID, model.UpdateCareerProfileRequest{
		CVMarkdown: ptr(jd),
		Identity:   &model.CareerIdentity{FullName: "Ada"},
		Targets:    &model.CareerTargets{Seniority: "head", Titles: []string{"Head of AI"}},
	})
	res, err := svc.Evaluate(context.Background(), orgID, userID, model.EvaluateCareerRequest{JDText: jd, Mode: model.CareerEvalModeFull})
	if err != nil {
		t.Fatal(err)
	}
	if res.Evaluation.Score.Overall < model.RecommendApplyMin {
		t.Fatalf("expected high score with identical CV/JD, got %.2f", res.Evaluation.Score.Overall)
	}
	kinds := map[string]bool{}
	for _, a := range res.Artifacts {
		kinds[a.Kind] = true
	}
	if !kinds[model.CareerArtifactCover] {
		t.Fatal("expected auto cover letter at ≥ 4.0")
	}
	if res.Evaluation.Score.RecommendFormAnswers && !kinds[model.CareerArtifactAnswers] {
		t.Fatal("expected Block H answers artifact at ≥ 4.5")
	}
}

func TestOfferPrepNotLegalAdvice(t *testing.T) {
	svc, _, orgID, userID := setupCareer(t)
	res, err := svc.Evaluate(context.Background(), orgID, userID, model.EvaluateCareerRequest{JDText: career.GoldenJDForTest()})
	if err != nil {
		t.Fatal(err)
	}
	prep, err := svc.OfferPrep(context.Background(), orgID, userID, res.Application.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !prep.NotLegalAdvice || !strings.Contains(strings.ToLower(prep.PrepMarkdown), "not legal advice") {
		t.Fatal("offer prep must disclaim legal advice")
	}
}

func TestLinkedInDraftCap(t *testing.T) {
	svc, _, orgID, userID := setupCareer(t)
	c, err := svc.AddContact(context.Background(), orgID, userID, model.AddCareerContactRequest{
		Name: "Alex Recruiter", Role: "Recruiter", Company: "Northwind Labs",
	})
	if err != nil {
		t.Fatal(err)
	}
	if c.LinkedInDraft == "" {
		t.Fatal("expected draft")
	}
	if len([]rune(c.LinkedInDraft)) > 300 {
		t.Fatalf("linkedin draft must be ≤300 runes, got %d", len([]rune(c.LinkedInDraft)))
	}
}

func TestUpsertStoryRejectsForeignID(t *testing.T) {
	svc, _, orgA, userA := setupCareer(t)
	orgB, userB := uuid.New(), uuid.New()
	mine, err := svc.UpsertStory(context.Background(), orgA, userA, model.CareerStory{Title: "Ada ship"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.UpsertStory(context.Background(), orgB, userB, model.CareerStory{
		ID: mine.ID, Title: "overwritten",
	})
	if err != ErrCareerNotFound {
		t.Fatalf("foreign story id must 404, got %v", err)
	}
	got, err := svc.ListStories(context.Background(), orgA, userA)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Title != "Ada ship" {
		t.Fatalf("owner story mutated: %+v", got)
	}
}

func TestBatchEvaluateSkipsBlacklist(t *testing.T) {
	svc, mem, orgID, userID := setupCareer(t)
	p, err := svc.GetOrCreateProfile(context.Background(), orgID, userID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddBlacklist(context.Background(), orgID, userID, model.AddCareerBlacklistRequest{
		Domain: "blocked.example",
	}); err != nil {
		t.Fatal(err)
	}
	_ = mem.UpsertPipelineItem(context.Background(), &model.CareerPipelineItem{
		OrgID: orgID, ProfileID: p.ID, ListingURL: "https://jobs.blocked.example/1",
		Company: "Blocked Co", Status: model.CareerPipelineOpen,
	})
	_ = mem.UpsertPipelineItem(context.Background(), &model.CareerPipelineItem{
		OrgID: orgID, ProfileID: p.ID, ListingURL: "https://jobs.ok.example/2",
		Company: "Ok Co", Status: model.CareerPipelineOpen,
	})
	out, err := svc.BatchEvaluate(context.Background(), orgID, userID, 8, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.Evaluated != 1 {
		t.Fatalf("evaluated=%d want 1 (blacklist must not count as success)", out.Evaluated)
	}
	if out.Skipped < 1 {
		t.Fatalf("skipped=%d want at least the blacklist hit", out.Skipped)
	}
	for _, r := range out.Results {
		if r.BlacklistHit != nil || r.Evaluation == nil {
			t.Fatal("batch must not return unconfirmed blacklist hits as evaluations")
		}
	}
}

func TestBatchEvaluateHonoursSelectedURLs(t *testing.T) {
	svc, mem, orgID, userID := setupCareer(t)
	p, err := svc.GetOrCreateProfile(context.Background(), orgID, userID)
	if err != nil {
		t.Fatal(err)
	}
	_ = mem.UpsertPipelineItem(context.Background(), &model.CareerPipelineItem{
		OrgID: orgID, ProfileID: p.ID, ListingURL: "https://jobs.one.example/1",
		Company: "One", Status: model.CareerPipelineOpen,
	})
	_ = mem.UpsertPipelineItem(context.Background(), &model.CareerPipelineItem{
		OrgID: orgID, ProfileID: p.ID, ListingURL: "https://jobs.two.example/2",
		Company: "Two", Status: model.CareerPipelineOpen,
	})
	out, err := svc.BatchEvaluate(context.Background(), orgID, userID, 8, []string{"https://jobs.one.example/1"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Evaluated+out.Skipped != 1 {
		t.Fatalf("selected URLs should score only those URLs, got evaluated=%d skipped=%d", out.Evaluated, out.Skipped)
	}
}

func TestPreviewListingRequiresURL(t *testing.T) {
	svc, _, orgID, userID := setupCareer(t)
	_, err := svc.PreviewListing(context.Background(), orgID, userID, "  ")
	if err != ErrCareerMissingInput {
		t.Fatalf("empty URL: %v", err)
	}
}

type previewFetch struct{}

func (previewFetch) Fetch(_ context.Context, rawURL string) (*research.Document, error) {
	return &research.Document{
		Source: research.Source{URL: rawURL, Title: "Staff Engineer"},
		Text:   "Company: Northwind Labs\n\nWe are hiring a Staff Engineer to lead inference.",
	}, nil
}

func TestPreviewListingReturnsJD(t *testing.T) {
	repo := newCareerMem()
	agents := &careerTestAgents{byBuiltin: map[string]*model.Agent{}}
	svc := NewCareerService(repo, agents, previewFetch{}, nil, nil, zap.NewNop()).(*careerService)
	orgID, userID := uuid.New(), uuid.New()
	out, err := svc.PreviewListing(context.Background(), orgID, userID, "https://jobs.example/staff")
	if err != nil {
		t.Fatal(err)
	}
	if out.Title != "Staff Engineer" || !strings.Contains(out.JDText, "inference") {
		t.Fatalf("preview: %+v", out)
	}
	if !out.Live {
		t.Fatal("fixture listing should be live")
	}
}

func TestBuiltinPortalsCoverAllBoards(t *testing.T) {
	list := career.BuiltinPortals()
	if len(list) < 80 {
		t.Fatalf("builtin portals=%d, want the CareerOps ATS list", len(list))
	}
	seen := map[string]bool{}
	boards := map[string]int{}
	for _, p := range list {
		if p.Board == "" || p.Slug == "" || p.Company == "" {
			t.Fatalf("incomplete seed: %+v", p)
		}
		key := p.Board + ":" + strings.ToLower(p.Slug)
		if seen[key] {
			t.Fatalf("duplicate seed %s", key)
		}
		seen[key] = true
		boards[p.Board]++
	}
	for _, want := range []string{"greenhouse", "ashby", "lever"} {
		if boards[want] == 0 {
			t.Fatalf("seed missing board %s", want)
		}
	}
}

func TestListPortalsSeedsBuiltinOnce(t *testing.T) {
	svc, _, orgID, userID := setupCareer(t)
	first, err := svc.ListPortals(context.Background(), orgID, userID)
	if err != nil {
		t.Fatal(err)
	}
	want := len(career.BuiltinPortals())
	if len(first) != want {
		t.Fatalf("portals=%d want %d", len(first), want)
	}
	second, err := svc.ListPortals(context.Background(), orgID, userID)
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != want {
		t.Fatalf("re-list duplicated seeds: %d", len(second))
	}
}

func TestEnsureSeedDoesNotOverwriteUserPortal(t *testing.T) {
	svc, _, orgID, userID := setupCareer(t)
	_, err := svc.AddPortal(context.Background(), orgID, userID, model.AddCareerPortalRequest{
		Board: "greenhouse", Slug: "anthropic", Company: "My Anthropic",
	})
	if err != nil {
		t.Fatal(err)
	}
	ports, err := svc.ListPortals(context.Background(), orgID, userID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range ports {
		if p.Board == "greenhouse" && p.Slug == "anthropic" {
			found = true
			if p.Company != "My Anthropic" {
				t.Fatalf("seed overwrote user company: %s", p.Company)
			}
		}
	}
	if !found {
		t.Fatal("user portal missing after seed")
	}
}

func TestScanAllUsesSeededPortalsAndAllBoards(t *testing.T) {
	svc, _, orgID, userID := setupCareer(t)
	var mu sync.Mutex
	calls := 0
	boards := map[string]int{}
	svc.scanBoard = func(_ context.Context, _ *http.Client, board, slug, company string) ([]career.PostedJob, error) {
		mu.Lock()
		calls++
		boards[board]++
		mu.Unlock()
		return []career.PostedJob{{
			URL: "https://jobs.example/" + board + "/" + slug, Company: company,
			Title: "Staff engineer", Board: board,
		}}, nil
	}
	out, err := svc.Scan(context.Background(), orgID, userID, model.ScanCareerRequest{})
	if err != nil {
		t.Fatal(err)
	}
	want := len(career.BuiltinPortals())
	if calls != want {
		t.Fatalf("scanned %d portals, want %d", calls, want)
	}
	if boards["greenhouse"] == 0 || boards["ashby"] == 0 || boards["lever"] == 0 {
		t.Fatalf("scan-all missed a board: %+v", boards)
	}
	if out.Run == nil || out.Run.Added != want {
		t.Fatalf("added=%v want %d", out.Run, want)
	}
}

func TestScanOneSlugTriesAllBoards(t *testing.T) {
	svc, _, orgID, userID := setupCareer(t)
	var mu sync.Mutex
	seen := []string{}
	svc.scanBoard = func(_ context.Context, _ *http.Client, board, slug, _ string) ([]career.PostedJob, error) {
		mu.Lock()
		seen = append(seen, board)
		mu.Unlock()
		if slug != "acme" {
			t.Errorf("slug=%s", slug)
		}
		return []career.PostedJob{{
			URL: "https://jobs.example/" + board + "/acme", Company: "Acme",
			Title: "Engineer", Board: board,
		}}, nil
	}
	out, err := svc.Scan(context.Background(), orgID, userID, model.ScanCareerRequest{Board: "all", Slug: "acme"})
	if err != nil {
		t.Fatal(err)
	}
	if len(seen) != 3 {
		t.Fatalf("boards tried=%v want greenhouse, ashby, lever", seen)
	}
	if out.Run == nil || out.Run.Added != 3 {
		t.Fatalf("added=%v want 3", out.Run)
	}
}

func ptr[T any](v T) *T { return &v }
