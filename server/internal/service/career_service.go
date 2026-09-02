package service

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/career"
	"github.com/jobshout/server/internal/llm"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
	"github.com/jobshout/server/internal/research"
)

var (
	ErrCareerNotFound       = errors.New("career: not found")
	ErrCareerBadStatus      = errors.New("career: illegal status change")
	ErrCareerMissingInput   = errors.New("career: job URL or JD text is required")
	ErrCareerEmptyBlacklist = errors.New("career: company or domain is required")
	ErrCareerBadUpload      = errors.New("career: unsupported or unreadable CV file")
)

// CareerService is the person-scoped career specialist.
type CareerService interface {
	EnsureCareerOps(ctx context.Context, orgID uuid.UUID) (*model.Agent, error)

	GetOrCreateProfile(ctx context.Context, orgID, userID uuid.UUID) (*model.CareerProfile, error)
	UpdateProfile(ctx context.Context, orgID, userID uuid.UUID, req model.UpdateCareerProfileRequest) (*model.CareerProfile, error)
	Intake(ctx context.Context, orgID, userID uuid.UUID, document string) (model.CareerIntakeProposal, error)
	UploadCV(ctx context.Context, orgID, userID uuid.UUID, filename, contentType string, data []byte) (model.CareerIntakeProposal, error)

	Evaluate(ctx context.Context, orgID, userID uuid.UUID, req model.EvaluateCareerRequest) (*model.CareerEvaluateResult, error)
	GetEvaluation(ctx context.Context, orgID, userID, id uuid.UUID) (*model.CareerEvaluation, error)
	ListEvaluations(ctx context.Context, orgID, userID uuid.UUID, pagination model.PaginationParams) (*model.PaginatedResponse[model.CareerEvaluation], error)

	ListPipeline(ctx context.Context, orgID, userID uuid.UUID, pagination model.PaginationParams) (*model.PaginatedResponse[model.CareerPipelineItem], error)
	ListTracker(ctx context.Context, orgID, userID uuid.UUID, status string, pagination model.PaginationParams) (*model.PaginatedResponse[model.CareerApplication], error)
	GetApplication(ctx context.Context, orgID, userID, id uuid.UUID) (*model.CareerApplication, error)
	SetStatus(ctx context.Context, orgID, userID, applicationID uuid.UUID, status, note string) (*model.CareerApplication, error)

	Scan(ctx context.Context, orgID, userID uuid.UUID, req model.ScanCareerRequest) (*model.CareerScanResult, error)
	ListPortals(ctx context.Context, orgID, userID uuid.UUID) ([]model.CareerPortal, error)
	AddPortal(ctx context.Context, orgID, userID uuid.UUID, req model.AddCareerPortalRequest) (*model.CareerPortal, error)
	PreviewListing(ctx context.Context, orgID, userID uuid.UUID, jobURL string) (*model.CareerListingPreview, error)

	AddBlacklist(ctx context.Context, orgID, userID uuid.UUID, req model.AddCareerBlacklistRequest) (*model.CareerBlacklistEntry, error)
	ListBlacklist(ctx context.Context, orgID, userID uuid.UUID) ([]model.CareerBlacklistEntry, error)

	TailorCV(ctx context.Context, orgID, userID, evaluationID uuid.UUID) (*model.CareerArtifact, error)
	CoverLetter(ctx context.Context, orgID, userID, evaluationID uuid.UUID) (*model.CareerArtifact, error)
	EmailDraft(ctx context.Context, orgID, userID, evaluationID uuid.UUID) (*model.CareerArtifact, error)
	ListArtifacts(ctx context.Context, orgID, userID, applicationID uuid.UUID) ([]model.CareerArtifact, error)
	ArtifactPDF(ctx context.Context, orgID, userID, artifactID uuid.UUID) (*model.CareerArtifact, []byte, error)

	Doctor(ctx context.Context, orgID, userID uuid.UUID) (model.CareerDoctorReport, error)
	Patterns(ctx context.Context, orgID, userID uuid.UUID) (model.CareerPatterns, error)
	Upskill(ctx context.Context, orgID, userID uuid.UUID) (model.CareerPatterns, error)
	Followup(ctx context.Context, orgID, userID, applicationID uuid.UUID) (*model.CareerFollowup, error)
	ListFollowups(ctx context.Context, orgID, userID uuid.UUID) ([]model.CareerFollowup, error)
	InterviewPrep(ctx context.Context, orgID, userID, applicationID uuid.UUID) (model.CareerInterviewPrep, error)
	MatchStories(ctx context.Context, orgID, userID, evaluationID uuid.UUID) ([]model.CareerStory, error)
	OfferPrep(ctx context.Context, orgID, userID, applicationID uuid.UUID) (model.CareerOfferPrep, error)
	SalaryGap(ctx context.Context, orgID, userID, applicationID uuid.UUID, advertised, actual string) (model.CareerSalaryGap, error)
	Deep(ctx context.Context, orgID, userID uuid.UUID, company, angle string) (map[string]any, error)
	ListStories(ctx context.Context, orgID, userID uuid.UUID) ([]model.CareerStory, error)
	UpsertStory(ctx context.Context, orgID, userID uuid.UUID, s model.CareerStory) (*model.CareerStory, error)
	AddContact(ctx context.Context, orgID, userID uuid.UUID, req model.AddCareerContactRequest) (*model.CareerContact, error)
	ListContacts(ctx context.Context, orgID, userID uuid.UUID) ([]model.CareerContact, error)
	BatchEvaluate(ctx context.Context, orgID, userID uuid.UUID, limit int, urls []string) (*model.CareerBatchResult, error)
}

type careerService struct {
	repo      repository.CareerRepository
	agentRepo repository.AgentRepository
	fetcher   career.Fetcher
	httpc     *http.Client
	llm       llm.Client
	research  ResearchService
	logger    *zap.Logger
	scanBoard func(ctx context.Context, httpc *http.Client, board, slug, company string) ([]career.PostedJob, error)
}

func NewCareerService(
	repo repository.CareerRepository,
	agentRepo repository.AgentRepository,
	fetcher career.Fetcher,
	llmClient llm.Client,
	researchSvc ResearchService,
	logger *zap.Logger,
) CareerService {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &careerService{
		repo: repo, agentRepo: agentRepo, fetcher: fetcher,
		httpc: &http.Client{Timeout: 20 * time.Second},
		llm:   llmClient, research: researchSvc, logger: logger,
	}
}

func careerOpsSeed(orgID uuid.UUID) *model.Agent { return career.Seed(orgID) }

func (s *careerService) EnsureCareerOps(ctx context.Context, orgID uuid.UUID) (*model.Agent, error) {
	existing, err := s.agentRepo.FindBuiltin(ctx, orgID, model.BuiltinCareerOps)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		if existing.Name != model.AgentNameCareerOps || existing.Role != "Career Agent" {
			existing.Name = model.AgentNameCareerOps
			existing.Role = "Career Agent"
			if err := s.agentRepo.Update(ctx, existing); err != nil {
				return nil, fmt.Errorf("career_svc: rename: %w", err)
			}
		}
		return existing, nil
	}
	agent := careerOpsSeed(orgID)
	if err := s.agentRepo.Create(ctx, agent); err != nil {
		return nil, fmt.Errorf("career_svc: seed: %w", err)
	}
	s.logger.Info("career: seeded Career Agent",
		zap.String("org_id", orgID.String()), zap.String("agent_id", agent.ID.String()))
	return agent, nil
}

func (s *careerService) gen() career.Generator {
	return career.GeneratorFromLLM(s.llm, "")
}

func (s *careerService) GetOrCreateProfile(ctx context.Context, orgID, userID uuid.UUID) (*model.CareerProfile, error) {
	p, err := s.repo.GetProfileByUser(ctx, orgID, userID)
	if err != nil {
		return nil, err
	}
	if p != nil {
		return p, nil
	}
	p = &model.CareerProfile{OrgID: orgID, UserID: userID}
	if err := s.repo.UpsertProfile(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

func (s *careerService) UpdateProfile(ctx context.Context, orgID, userID uuid.UUID, req model.UpdateCareerProfileRequest) (*model.CareerProfile, error) {
	p, err := s.GetOrCreateProfile(ctx, orgID, userID)
	if err != nil {
		return nil, err
	}
	oldCV := p.CVMarkdown
	if req.CVMarkdown != nil {
		p.CVMarkdown = *req.CVMarkdown
	}
	if req.Identity != nil {
		p.Identity = *req.Identity
	}
	if req.Targets != nil {
		p.Targets = *req.Targets
	}
	if req.Location != nil {
		p.Location = *req.Location
	}
	if req.WorkAuth != nil {
		p.WorkAuth = *req.WorkAuth
	}
	if req.Voice != nil {
		p.Voice = *req.Voice
	}
	if req.HouseRules != nil {
		p.HouseRules = *req.HouseRules
	}
	if req.ProofPoints != nil {
		p.ProofPoints = *req.ProofPoints
	}
	if req.Narrative != nil {
		p.Narrative = *req.Narrative
	}
	if err := s.repo.UpsertProfile(ctx, p); err != nil {
		return nil, err
	}
	if req.CVMarkdown != nil && *req.CVMarkdown != oldCV {
		_ = s.repo.InsertProfileVersion(ctx, orgID, p.ID, p.CVMarkdown, "profile update")
	}
	return p, nil
}

func (s *careerService) Intake(ctx context.Context, orgID, userID uuid.UUID, document string) (model.CareerIntakeProposal, error) {
	if _, err := s.GetOrCreateProfile(ctx, orgID, userID); err != nil {
		return model.CareerIntakeProposal{}, err
	}
	return career.ProposeIntake(document), nil
}

func (s *careerService) UploadCV(ctx context.Context, orgID, userID uuid.UUID, filename, contentType string, data []byte) (model.CareerIntakeProposal, error) {
	p, err := s.GetOrCreateProfile(ctx, orgID, userID)
	if err != nil {
		return model.CareerIntakeProposal{}, err
	}
	text, err := career.ExtractCVMarkdown(filename, contentType, data)
	if err != nil {
		return model.CareerIntakeProposal{}, fmt.Errorf("%w: %s", ErrCareerBadUpload, err.Error())
	}
	doc := &model.CareerDocument{
		OrgID: orgID, ProfileID: p.ID,
		Filename: filename, ContentType: "application/pdf", Body: text,
	}
	if err := s.repo.InsertDocument(ctx, doc); err != nil {
		return model.CareerIntakeProposal{}, err
	}
	oldCV := p.CVMarkdown
	p.CVMarkdown = text
	prop := career.ProposeIntake(text)
	if prop.Patch.Identity != nil && p.Identity.FullName == "" && strings.TrimSpace(prop.Patch.Identity.FullName) != "" {
		p.Identity.FullName = strings.TrimSpace(prop.Patch.Identity.FullName)
	}
	if err := s.repo.UpsertProfile(ctx, p); err != nil {
		return model.CareerIntakeProposal{}, err
	}
	if text != oldCV {
		_ = s.repo.InsertProfileVersion(ctx, orgID, p.ID, p.CVMarkdown, "pdf upload")
	}
	prop.Patch.CVMarkdown = &text
	return prop, nil
}

func (s *careerService) Evaluate(ctx context.Context, orgID, userID uuid.UUID, req model.EvaluateCareerRequest) (*model.CareerEvaluateResult, error) {
	if strings.TrimSpace(req.JobURL) == "" && strings.TrimSpace(req.JDText) == "" {
		return nil, ErrCareerMissingInput
	}
	if _, err := s.EnsureCareerOps(ctx, orgID); err != nil {
		s.logger.Warn("career: could not seed agent", zap.Error(err))
	}
	profile, err := s.GetOrCreateProfile(ctx, orgID, userID)
	if err != nil {
		return nil, err
	}
	listing, err := career.Extract(ctx, s.fetcher, s.httpc, req.JobURL, req.JDText)
	if err != nil {
		return nil, err
	}

	pipe := &model.CareerPipelineItem{
		OrgID: orgID, ProfileID: profile.ID,
		ListingURL: nzURL(listing), Company: listing.Company, Title: listing.Title,
		Source: listing.Via, Status: model.CareerPipelineOpen, Liveness: "live",
	}
	now := time.Now()
	pipe.LivenessCheckedAt = &now
	if !listing.Live {
		pipe.Status = model.CareerPipelineClosed
		pipe.Liveness = "dead"
		if pipe.ListingURL != "" {
			_ = s.repo.UpsertPipelineItem(ctx, pipe)
		}
		return &model.CareerEvaluateResult{Dead: true, DeadReason: listing.DeadReason}, nil
	}

	bl, err := s.repo.ListBlacklist(ctx, orgID, profile.ID)
	if err != nil {
		return nil, err
	}
	if hit := career.MatchBlacklist(listing.Company, listing.URL, bl); hit != nil && !req.ConfirmBlacklist {
		return &model.CareerEvaluateResult{BlacklistHit: hit}, nil
	}

	mode := req.Mode
	if mode != model.CareerEvalModeTriage {
		mode = model.CareerEvalModeFull
	}
	ev, err := career.EvaluateBlocks(ctx, listing, profile, mode, s.gen())
	if err != nil {
		return nil, err
	}
	ev.OrgID = orgID
	ev.ProfileID = profile.ID

	if pipe.ListingURL != "" {
		if err := s.repo.UpsertPipelineItem(ctx, pipe); err != nil {
			return nil, err
		}
		ev.PipelineItemID = &pipe.ID
	}

	appStatus := model.CareerStatusEvaluated
	if ev.HardStop {
		appStatus = model.CareerStatusSkip
	}
	score := ev.Score.Overall
	app := &model.CareerApplication{
		OrgID: orgID, ProfileID: profile.ID,
		Company: ev.Company, Role: ev.Role, ListingURL: listing.URL,
		Status: appStatus, Score: &score, Via: listing.Via, Agency: listing.Agency,
	}
	if existing, _ := s.repo.GetApplicationByURL(ctx, profile.ID, listing.URL); existing != nil && listing.URL != "" {
		app.ID = existing.ID
		app.Status = existing.Status
		if ev.HardStop {
			app.Status = model.CareerStatusSkip
		}
	}
	if err := s.repo.UpsertApplication(ctx, app); err != nil {
		return nil, err
	}
	ev.ApplicationID = &app.ID
	if err := s.repo.InsertEvaluation(ctx, ev); err != nil {
		return nil, err
	}

	out := &model.CareerEvaluateResult{Evaluation: ev, Application: app}
	if ev.Blocks.H != "" && ev.Score.RecommendFormAnswers {
		if art, err := s.persistArtifact(ctx, profile, app, ev, model.CareerArtifactAnswers, "Form answers (draft only)", func() (string, error) {
			return ev.Blocks.H, nil
		}); err == nil {
			out.Artifacts = append(out.Artifacts, *art)
		}
	}
	if req.TailorCV && strings.TrimSpace(profile.CVMarkdown) != "" {
		if art, err := s.persistArtifact(ctx, profile, app, ev, model.CareerArtifactCV, "Tailored CV", func() (string, error) {
			return career.TailorCV(ctx, listing, profile, ev, s.gen())
		}); err == nil {
			out.Artifacts = append(out.Artifacts, *art)
		}
	}
	if ev.Score.RecommendApply {
		if art, err := s.persistArtifact(ctx, profile, app, ev, model.CareerArtifactCover, "Cover letter", func() (string, error) {
			return career.DraftCoverLetter(ctx, listing, profile, ev, s.gen())
		}); err == nil {
			out.Artifacts = append(out.Artifacts, *art)
		}
	}
	if stub := career.StoryFromEval(ev); stub != nil {
		existing, _ := s.repo.ListStories(ctx, orgID, profile.ID)
		dup := false
		for _, st := range existing {
			if st.Title == stub.Title {
				dup = true
				break
			}
		}
		if !dup {
			stub.OrgID = orgID
			stub.ProfileID = profile.ID
			_ = s.repo.UpsertStory(ctx, stub)
		}
	}
	return out, nil
}

func (s *careerService) persistArtifact(
	ctx context.Context, profile *model.CareerProfile, app *model.CareerApplication, ev *model.CareerEvaluation,
	kind, title string, bodyFn func() (string, error),
) (*model.CareerArtifact, error) {
	body, err := bodyFn()
	if err != nil {
		return nil, err
	}
	a := &model.CareerArtifact{
		OrgID: profile.OrgID, ProfileID: profile.ID,
		ApplicationID: &app.ID, EvaluationID: &ev.ID,
		Kind: kind, Title: title, BodyMarkdown: body,
	}
	if kind == model.CareerArtifactCV {
		attachCVPDF(a, profile, ev)
	}
	if err := s.repo.InsertArtifact(ctx, a); err != nil {
		return nil, err
	}
	return a, nil
}

func attachCVPDF(a *model.CareerArtifact, profile *model.CareerProfile, ev *model.CareerEvaluation) {
	name := ""
	if profile != nil {
		name = profile.Identity.FullName
	}
	company, role := "", ""
	if ev != nil {
		company, role = ev.Company, ev.Role
	}
	pdf, err := career.MarkdownToPDF(name, a.BodyMarkdown)
	if err != nil || len(pdf) == 0 {
		return
	}
	a.FileBytes = pdf
	a.HasPDF = true
	a.PDFBase64 = base64.StdEncoding.EncodeToString(pdf)
	a.PDFFilename = career.PDFFilename(name, company, role)
}

func (s *careerService) ArtifactPDF(ctx context.Context, orgID, userID, artifactID uuid.UUID) (*model.CareerArtifact, []byte, error) {
	p, err := s.GetOrCreateProfile(ctx, orgID, userID)
	if err != nil {
		return nil, nil, err
	}
	a, err := s.repo.GetArtifact(ctx, orgID, artifactID)
	if err != nil {
		return nil, nil, err
	}
	if a == nil || a.ProfileID != p.ID {
		return nil, nil, ErrCareerNotFound
	}
	pdf := a.FileBytes
	if len(pdf) == 0 && a.Kind == model.CareerArtifactCV && strings.TrimSpace(a.BodyMarkdown) != "" {
		if built, err := career.MarkdownToPDF(p.Identity.FullName, a.BodyMarkdown); err == nil {
			pdf = built
		}
	}
	if len(pdf) == 0 {
		return nil, nil, ErrCareerNotFound
	}
	if a.PDFFilename == "" {
		a.PDFFilename = career.PDFFilename(p.Identity.FullName, "", a.Title)
	}
	return a, pdf, nil
}

func nzURL(l *career.JobListing) string {
	if l == nil {
		return ""
	}
	if l.URL != "" {
		return l.URL
	}
	return ""
}

func (s *careerService) GetEvaluation(ctx context.Context, orgID, userID, id uuid.UUID) (*model.CareerEvaluation, error) {
	p, err := s.GetOrCreateProfile(ctx, orgID, userID)
	if err != nil {
		return nil, err
	}
	e, err := s.repo.GetEvaluation(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	if e == nil || e.ProfileID != p.ID {
		return nil, ErrCareerNotFound
	}
	return e, nil
}

func (s *careerService) ListEvaluations(ctx context.Context, orgID, userID uuid.UUID, pagination model.PaginationParams) (*model.PaginatedResponse[model.CareerEvaluation], error) {
	p, err := s.GetOrCreateProfile(ctx, orgID, userID)
	if err != nil {
		return nil, err
	}
	return s.repo.ListEvaluations(ctx, orgID, p.ID, pagination)
}

func (s *careerService) ListPipeline(ctx context.Context, orgID, userID uuid.UUID, pagination model.PaginationParams) (*model.PaginatedResponse[model.CareerPipelineItem], error) {
	p, err := s.GetOrCreateProfile(ctx, orgID, userID)
	if err != nil {
		return nil, err
	}
	return s.repo.ListPipeline(ctx, orgID, p.ID, pagination)
}

func (s *careerService) ListTracker(ctx context.Context, orgID, userID uuid.UUID, status string, pagination model.PaginationParams) (*model.PaginatedResponse[model.CareerApplication], error) {
	p, err := s.GetOrCreateProfile(ctx, orgID, userID)
	if err != nil {
		return nil, err
	}
	return s.repo.ListApplications(ctx, orgID, p.ID, status, pagination)
}

func (s *careerService) GetApplication(ctx context.Context, orgID, userID, id uuid.UUID) (*model.CareerApplication, error) {
	p, err := s.GetOrCreateProfile(ctx, orgID, userID)
	if err != nil {
		return nil, err
	}
	a, err := s.repo.GetApplication(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	if a == nil || a.ProfileID != p.ID {
		return nil, ErrCareerNotFound
	}
	return a, nil
}

func (s *careerService) SetStatus(ctx context.Context, orgID, userID, applicationID uuid.UUID, status, note string) (*model.CareerApplication, error) {
	if !career.KnownStatus(status) {
		return nil, ErrCareerBadStatus
	}
	a, err := s.GetApplication(ctx, orgID, userID, applicationID)
	if err != nil {
		return nil, err
	}
	if err := career.ValidateTransition(a.Status, status); err != nil {
		return nil, ErrCareerBadStatus
	}
	from := a.Status
	a.Status = status
	if err := s.repo.UpdateApplication(ctx, a); err != nil {
		return nil, err
	}
	_ = s.repo.AppendStatusEvent(ctx, &model.CareerStatusEvent{
		OrgID: orgID, ProfileID: a.ProfileID, ApplicationID: a.ID,
		FromStatus: from, ToStatus: status, Note: note,
	})
	if from != model.CareerStatusApplied && status == model.CareerStatusApplied {
		due := time.Now().Add(7 * 24 * time.Hour)
		_ = s.repo.InsertFollowup(ctx, &model.CareerFollowup{
			OrgID: orgID, ProfileID: a.ProfileID, ApplicationID: a.ID,
			DueAt: due, Kind: "followup", Draft: career.FollowupDraft(a, due),
		})
	}
	return a, nil
}

func (s *careerService) Scan(ctx context.Context, orgID, userID uuid.UUID, req model.ScanCareerRequest) (*model.CareerScanResult, error) {
	profile, err := s.GetOrCreateProfile(ctx, orgID, userID)
	if err != nil {
		return nil, err
	}
	if err := s.ensureSeedPortals(ctx, orgID, profile.ID); err != nil {
		s.logger.Warn("career: seed portals", zap.Error(err))
	}
	run := &model.CareerScanRun{OrgID: orgID, ProfileID: profile.ID, Status: "running"}
	if err := s.repo.InsertScanRun(ctx, run); err != nil {
		return nil, err
	}

	portals := []model.CareerPortal{}
	slug := strings.TrimSpace(req.Slug)
	board := strings.ToLower(strings.TrimSpace(req.Board))
	if slug != "" {
		boards := []string{board}
		if board == "" || board == "all" {
			boards = []string{"greenhouse", "ashby", "lever"}
		}
		include := append([]string{}, profile.Targets.Titles...)
		if q := strings.TrimSpace(req.Query); q != "" {
			include = append(include, q)
		}
		for _, b := range boards {
			portals = append(portals, model.CareerPortal{
				Board: b, Slug: slug, Company: req.Company, Enabled: true,
				TitleInclude: include,
			})
		}
	} else {
		stored, err := s.repo.ListPortals(ctx, orgID, profile.ID)
		if err != nil {
			return nil, err
		}
		for _, p := range stored {
			if !p.Enabled {
				continue
			}
			if board != "" && board != "all" && p.Board != board {
				continue
			}
			if len(p.TitleInclude) == 0 {
				p.TitleInclude = profile.Targets.Titles
			}
			portals = append(portals, p)
		}
	}

	scanFn := s.scanBoard
	if scanFn == nil {
		scanFn = career.ScanBoard
	}
	scanClient := &http.Client{Timeout: 12 * time.Second}

	added := []model.CareerPipelineItem{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	for _, portal := range portals {
		portal := portal
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case <-ctx.Done():
				mu.Lock()
				run.Skipped++
				mu.Unlock()
				return
			case sem <- struct{}{}:
			}
			defer func() { <-sem }()

			jobs, err := scanFn(ctx, scanClient, portal.Board, portal.Slug, portal.Company)
			if err != nil {
				s.logger.Warn("career: scan board failed", zap.String("board", portal.Board), zap.String("slug", portal.Slug), zap.Error(err))
				mu.Lock()
				run.Skipped++
				mu.Unlock()
				return
			}
			for _, job := range jobs {
				if !career.TitleAllowed(job.Title, portal.TitleInclude, portal.TitleExclude) {
					mu.Lock()
					run.Skipped++
					mu.Unlock()
					continue
				}
				seen, _ := s.repo.HasScanEvent(ctx, profile.ID, job.URL)
				if seen {
					mu.Lock()
					run.Skipped++
					mu.Unlock()
					continue
				}
				item := &model.CareerPipelineItem{
					OrgID: orgID, ProfileID: profile.ID,
					ListingURL: job.URL, Company: job.Company, Title: job.Title,
					Source: job.Board, Status: model.CareerPipelineOpen, Liveness: "unknown",
				}
				if err := s.repo.UpsertPipelineItem(ctx, item); err != nil {
					mu.Lock()
					run.Skipped++
					mu.Unlock()
					continue
				}
				_ = s.repo.InsertScanEvent(ctx, orgID, profile.ID, job.URL, job.Company, job.Title)
				mu.Lock()
				run.Added++
				added = append(added, *item)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	now := time.Now()
	run.Status = "completed"
	run.CompletedAt = &now
	_ = s.repo.UpdateScanRun(ctx, run)
	return &model.CareerScanResult{Run: run, Added: added}, nil
}

func (s *careerService) ListPortals(ctx context.Context, orgID, userID uuid.UUID) ([]model.CareerPortal, error) {
	p, err := s.GetOrCreateProfile(ctx, orgID, userID)
	if err != nil {
		return nil, err
	}
	if err := s.ensureSeedPortals(ctx, orgID, p.ID); err != nil {
		return nil, err
	}
	return s.repo.ListPortals(ctx, orgID, p.ID)
}

func (s *careerService) ensureSeedPortals(ctx context.Context, orgID, profileID uuid.UUID) error {
	existing, err := s.repo.ListPortals(ctx, orgID, profileID)
	if err != nil {
		return err
	}
	have := make(map[string]bool, len(existing))
	for _, p := range existing {
		have[p.Board+":"+strings.ToLower(p.Slug)] = true
	}
	for _, seed := range career.BuiltinPortals() {
		key := seed.Board + ":" + strings.ToLower(seed.Slug)
		if have[key] {
			continue
		}
		portal := &model.CareerPortal{
			OrgID: orgID, ProfileID: profileID,
			Board: seed.Board, Slug: seed.Slug, Company: seed.Company, Enabled: true,
		}
		if err := s.repo.UpsertPortal(ctx, portal); err != nil {
			return err
		}
		have[key] = true
	}
	return nil
}

func (s *careerService) AddPortal(ctx context.Context, orgID, userID uuid.UUID, req model.AddCareerPortalRequest) (*model.CareerPortal, error) {
	p, err := s.GetOrCreateProfile(ctx, orgID, userID)
	if err != nil {
		return nil, err
	}
	portal := &model.CareerPortal{
		OrgID: orgID, ProfileID: p.ID,
		Board: strings.ToLower(strings.TrimSpace(req.Board)),
		Slug:  strings.TrimSpace(req.Slug), Company: req.Company,
		TitleInclude: req.TitleInclude, TitleExclude: req.TitleExclude, Enabled: true,
	}
	if portal.Board == "" {
		portal.Board = "greenhouse"
	}
	if portal.Board == "all" || (portal.Board != "greenhouse" && portal.Board != "ashby" && portal.Board != "lever") {
		return nil, fmt.Errorf("career: board must be greenhouse, ashby, or lever")
	}
	if portal.Slug == "" {
		return nil, fmt.Errorf("career: portal slug is required")
	}
	if err := s.repo.UpsertPortal(ctx, portal); err != nil {
		return nil, err
	}
	return portal, nil
}

func (s *careerService) AddBlacklist(ctx context.Context, orgID, userID uuid.UUID, req model.AddCareerBlacklistRequest) (*model.CareerBlacklistEntry, error) {
	if strings.TrimSpace(req.Company) == "" && strings.TrimSpace(req.Domain) == "" {
		return nil, ErrCareerEmptyBlacklist
	}
	p, err := s.GetOrCreateProfile(ctx, orgID, userID)
	if err != nil {
		return nil, err
	}
	e := &model.CareerBlacklistEntry{
		OrgID: orgID, ProfileID: p.ID, Company: req.Company, Domain: req.Domain, Reason: req.Reason,
	}
	if err := s.repo.InsertBlacklist(ctx, e); err != nil {
		return nil, err
	}
	return e, nil
}

func (s *careerService) ListBlacklist(ctx context.Context, orgID, userID uuid.UUID) ([]model.CareerBlacklistEntry, error) {
	p, err := s.GetOrCreateProfile(ctx, orgID, userID)
	if err != nil {
		return nil, err
	}
	return s.repo.ListBlacklist(ctx, orgID, p.ID)
}

func (s *careerService) loadEval(ctx context.Context, orgID, userID, evaluationID uuid.UUID) (*model.CareerProfile, *model.CareerEvaluation, *model.CareerApplication, error) {
	p, err := s.GetOrCreateProfile(ctx, orgID, userID)
	if err != nil {
		return nil, nil, nil, err
	}
	ev, err := s.GetEvaluation(ctx, orgID, userID, evaluationID)
	if err != nil {
		return nil, nil, nil, err
	}
	var app *model.CareerApplication
	if ev.ApplicationID != nil {
		app, err = s.GetApplication(ctx, orgID, userID, *ev.ApplicationID)
		if err != nil {
			return nil, nil, nil, err
		}
	}
	return p, ev, app, nil
}

func (s *careerService) TailorCV(ctx context.Context, orgID, userID, evaluationID uuid.UUID) (*model.CareerArtifact, error) {
	p, ev, app, err := s.loadEval(ctx, orgID, userID, evaluationID)
	if err != nil {
		return nil, err
	}
	if app == nil {
		return nil, ErrCareerNotFound
	}
	listing := &career.JobListing{URL: ev.ListingURL, Company: ev.Company, Title: ev.Role, Text: ev.JDText}
	return s.persistArtifact(ctx, p, app, ev, model.CareerArtifactCV, "Tailored CV", func() (string, error) {
		return career.TailorCV(ctx, listing, p, ev, s.gen())
	})
}

func (s *careerService) CoverLetter(ctx context.Context, orgID, userID, evaluationID uuid.UUID) (*model.CareerArtifact, error) {
	p, ev, app, err := s.loadEval(ctx, orgID, userID, evaluationID)
	if err != nil {
		return nil, err
	}
	if app == nil {
		return nil, ErrCareerNotFound
	}
	listing := &career.JobListing{URL: ev.ListingURL, Company: ev.Company, Title: ev.Role, Text: ev.JDText}
	return s.persistArtifact(ctx, p, app, ev, model.CareerArtifactCover, "Cover letter", func() (string, error) {
		return career.DraftCoverLetter(ctx, listing, p, ev, s.gen())
	})
}

func (s *careerService) EmailDraft(ctx context.Context, orgID, userID, evaluationID uuid.UUID) (*model.CareerArtifact, error) {
	p, ev, app, err := s.loadEval(ctx, orgID, userID, evaluationID)
	if err != nil {
		return nil, err
	}
	if app == nil {
		return nil, ErrCareerNotFound
	}
	subj, body, err := career.DraftEmail(ctx, p, ev, s.gen())
	if err != nil {
		return nil, err
	}
	a := &model.CareerArtifact{
		OrgID: p.OrgID, ProfileID: p.ID, ApplicationID: &app.ID, EvaluationID: &ev.ID,
		Kind: model.CareerArtifactEmail, Title: subj, BodyMarkdown: body,
	}
	if err := s.repo.InsertArtifact(ctx, a); err != nil {
		return nil, err
	}
	return a, nil
}

func (s *careerService) ListArtifacts(ctx context.Context, orgID, userID, applicationID uuid.UUID) ([]model.CareerArtifact, error) {
	if _, err := s.GetApplication(ctx, orgID, userID, applicationID); err != nil {
		return nil, err
	}
	return s.repo.ListArtifacts(ctx, applicationID)
}

func (s *careerService) Doctor(ctx context.Context, orgID, userID uuid.UUID) (model.CareerDoctorReport, error) {
	p, err := s.GetOrCreateProfile(ctx, orgID, userID)
	if err != nil {
		return model.CareerDoctorReport{}, err
	}
	apps, err := s.repo.ListApplications(ctx, orgID, p.ID, "", model.PaginationParams{Page: 1, PerPage: 100})
	if err != nil {
		return model.CareerDoctorReport{}, err
	}
	pipe, err := s.repo.ListPipeline(ctx, orgID, p.ID, model.PaginationParams{Page: 1, PerPage: 100})
	if err != nil {
		return model.CareerDoctorReport{}, err
	}
	stories, err := s.repo.ListStories(ctx, orgID, p.ID)
	if err != nil {
		return model.CareerDoctorReport{}, err
	}
	return career.Doctor(p, apps.Data, pipe.Data, stories), nil
}

func (s *careerService) Patterns(ctx context.Context, orgID, userID uuid.UUID) (model.CareerPatterns, error) {
	p, err := s.GetOrCreateProfile(ctx, orgID, userID)
	if err != nil {
		return model.CareerPatterns{}, err
	}
	apps, err := s.repo.ListApplications(ctx, orgID, p.ID, "", model.PaginationParams{Page: 1, PerPage: 200})
	if err != nil {
		return model.CareerPatterns{}, err
	}
	out := model.CareerPatterns{Applications: apps.Total, ByStatus: map[string]int{}}
	var sum float64
	var n int
	for _, a := range apps.Data {
		out.ByStatus[a.Status]++
		if a.Score != nil {
			sum += *a.Score
			n++
		}
	}
	if n > 0 {
		out.AvgScore = sum / float64(n)
	}
	evals, err := s.repo.ListEvaluations(ctx, orgID, p.ID, model.PaginationParams{Page: 1, PerPage: 80})
	if err == nil {
		out.SkillGaps = career.Upskill(evals.Data, p.CVMarkdown)
	}
	return out, nil
}

func (s *careerService) Upskill(ctx context.Context, orgID, userID uuid.UUID) (model.CareerPatterns, error) {
	return s.Patterns(ctx, orgID, userID)
}

func (s *careerService) Followup(ctx context.Context, orgID, userID, applicationID uuid.UUID) (*model.CareerFollowup, error) {
	a, err := s.GetApplication(ctx, orgID, userID, applicationID)
	if err != nil {
		return nil, err
	}
	due := time.Now().Add(7 * 24 * time.Hour)
	f := &model.CareerFollowup{
		OrgID: orgID, ProfileID: a.ProfileID, ApplicationID: a.ID,
		DueAt: due, Kind: "followup", Draft: career.FollowupDraft(a, due),
	}
	if err := s.repo.InsertFollowup(ctx, f); err != nil {
		return nil, err
	}
	return f, nil
}

func (s *careerService) ListFollowups(ctx context.Context, orgID, userID uuid.UUID) ([]model.CareerFollowup, error) {
	p, err := s.GetOrCreateProfile(ctx, orgID, userID)
	if err != nil {
		return nil, err
	}
	return s.repo.ListFollowups(ctx, orgID, p.ID)
}

func (s *careerService) latestEvalForApp(ctx context.Context, orgID, applicationID uuid.UUID) (*model.CareerEvaluation, error) {
	return s.repo.GetLatestEvaluationForApp(ctx, orgID, applicationID)
}

func (s *careerService) InterviewPrep(ctx context.Context, orgID, userID, applicationID uuid.UUID) (model.CareerInterviewPrep, error) {
	p, err := s.GetOrCreateProfile(ctx, orgID, userID)
	if err != nil {
		return model.CareerInterviewPrep{}, err
	}
	a, err := s.GetApplication(ctx, orgID, userID, applicationID)
	if err != nil {
		return model.CareerInterviewPrep{}, err
	}
	ev, _ := s.latestEvalForApp(ctx, orgID, applicationID)
	stories, err := s.repo.ListStories(ctx, orgID, p.ID)
	if err != nil {
		return model.CareerInterviewPrep{}, err
	}
	return career.InterviewPrep(a, ev, stories, p), nil
}

func (s *careerService) MatchStories(ctx context.Context, orgID, userID, evaluationID uuid.UUID) ([]model.CareerStory, error) {
	p, err := s.GetOrCreateProfile(ctx, orgID, userID)
	if err != nil {
		return nil, err
	}
	ev, err := s.GetEvaluation(ctx, orgID, userID, evaluationID)
	if err != nil {
		return nil, err
	}
	stories, err := s.repo.ListStories(ctx, orgID, p.ID)
	if err != nil {
		return nil, err
	}
	return career.MatchStories(stories, ev.JDText, ev.Role), nil
}

func (s *careerService) OfferPrep(ctx context.Context, orgID, userID, applicationID uuid.UUID) (model.CareerOfferPrep, error) {
	p, err := s.GetOrCreateProfile(ctx, orgID, userID)
	if err != nil {
		return model.CareerOfferPrep{}, err
	}
	a, err := s.GetApplication(ctx, orgID, userID, applicationID)
	if err != nil {
		return model.CareerOfferPrep{}, err
	}
	prep := career.OfferPrep(a, p)
	_ = s.repo.InsertOffer(ctx, &model.CareerOffer{
		OrgID: orgID, ProfileID: p.ID, ApplicationID: a.ID,
		Notes: "Draft offer walkthrough. Not legal advice.",
	})
	return prep, nil
}

func (s *careerService) SalaryGap(ctx context.Context, orgID, userID, applicationID uuid.UUID, advertised, actual string) (model.CareerSalaryGap, error) {
	p, err := s.GetOrCreateProfile(ctx, orgID, userID)
	if err != nil {
		return model.CareerSalaryGap{}, err
	}
	var appID *uuid.UUID
	if applicationID != uuid.Nil {
		if a, err := s.GetApplication(ctx, orgID, userID, applicationID); err != nil {
			return model.CareerSalaryGap{}, err
		} else {
			appID = &a.ID
		}
	}
	gap := career.SalaryGap(p, advertised, actual)
	_ = s.repo.InsertSalaryObservation(ctx, &model.CareerSalaryObservation{
		OrgID: orgID, ProfileID: p.ID, ApplicationID: appID,
		Desired: gap.Desired, Advertised: advertised, Actual: actual,
	})
	return gap, nil
}

func (s *careerService) Deep(ctx context.Context, orgID, userID uuid.UUID, company, angle string) (map[string]any, error) {
	if s.research == nil || !s.research.Available() {
		return map[string]any{"available": false, "message": "Research Agent is not configured."}, nil
	}
	topic := "Company research: " + company
	if angle != "" {
		topic += " — " + angle
	}
	brief, err := s.research.Research(ctx, orgID, research.Request{Topic: topic, Context: "CareerOps Block D / deep. Bounded company facts only."}, nil)
	if err != nil {
		return nil, err
	}
	return map[string]any{"brief": brief}, nil
}

func (s *careerService) ListStories(ctx context.Context, orgID, userID uuid.UUID) ([]model.CareerStory, error) {
	p, err := s.GetOrCreateProfile(ctx, orgID, userID)
	if err != nil {
		return nil, err
	}
	return s.repo.ListStories(ctx, orgID, p.ID)
}

func (s *careerService) UpsertStory(ctx context.Context, orgID, userID uuid.UUID, st model.CareerStory) (*model.CareerStory, error) {
	p, err := s.GetOrCreateProfile(ctx, orgID, userID)
	if err != nil {
		return nil, err
	}
	if st.ID != uuid.Nil {
		existing, err := s.repo.GetStoryByID(ctx, st.ID)
		if err != nil {
			return nil, err
		}
		if existing != nil && (existing.OrgID != orgID || existing.ProfileID != p.ID) {
			return nil, ErrCareerNotFound
		}
	}
	st.OrgID = orgID
	st.ProfileID = p.ID
	if err := s.repo.UpsertStory(ctx, &st); err != nil {
		return nil, err
	}
	return &st, nil
}

func (s *careerService) AddContact(ctx context.Context, orgID, userID uuid.UUID, req model.AddCareerContactRequest) (*model.CareerContact, error) {
	if strings.TrimSpace(req.Name) == "" && strings.TrimSpace(req.LinkedInURL) == "" {
		return nil, fmt.Errorf("career: contact name or LinkedIn URL is required")
	}
	p, err := s.GetOrCreateProfile(ctx, orgID, userID)
	if err != nil {
		return nil, err
	}
	c := &model.CareerContact{
		OrgID: orgID, ProfileID: p.ID, ApplicationID: req.ApplicationID,
		Name: req.Name, Role: req.Role, Company: req.Company,
		Email: req.Email, LinkedInURL: req.LinkedInURL, Note: req.Note,
	}
	c.LinkedInDraft = career.LinkedInDraft(req.Name, req.Role, req.Company, p)
	if utf8Count := len([]rune(c.LinkedInDraft)); utf8Count > 300 {
		c.LinkedInDraft = string([]rune(c.LinkedInDraft)[:300])
	}
	if err := s.repo.InsertContact(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

func (s *careerService) ListContacts(ctx context.Context, orgID, userID uuid.UUID) ([]model.CareerContact, error) {
	p, err := s.GetOrCreateProfile(ctx, orgID, userID)
	if err != nil {
		return nil, err
	}
	return s.repo.ListContacts(ctx, orgID, p.ID)
}

func (s *careerService) PreviewListing(ctx context.Context, orgID, userID uuid.UUID, jobURL string) (*model.CareerListingPreview, error) {
	jobURL = strings.TrimSpace(jobURL)
	if jobURL == "" {
		return nil, ErrCareerMissingInput
	}
	if _, err := s.GetOrCreateProfile(ctx, orgID, userID); err != nil {
		return nil, err
	}
	listing, err := career.Extract(ctx, s.fetcher, s.httpc, jobURL, "")
	if err != nil {
		return nil, err
	}
	return &model.CareerListingPreview{
		URL:        listing.URL,
		Company:    listing.Company,
		Title:      listing.Title,
		JDText:     listing.Text,
		Live:       listing.Live,
		DeadReason: listing.DeadReason,
	}, nil
}

func (s *careerService) BatchEvaluate(ctx context.Context, orgID, userID uuid.UUID, limit int, urls []string) (*model.CareerBatchResult, error) {
	if limit <= 0 || limit > 10 {
		limit = 8
	}
	toScore := []string{}
	mode := model.CareerEvalModeTriage
	for _, u := range urls {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		toScore = append(toScore, u)
		if len(toScore) >= limit {
			break
		}
	}
	if len(toScore) > 0 {
		mode = model.CareerEvalModeFull
	} else {
		pipe, err := s.ListPipeline(ctx, orgID, userID, model.PaginationParams{Page: 1, PerPage: 50})
		if err != nil {
			return nil, err
		}
		for _, it := range pipe.Data {
			if len(toScore) >= limit {
				break
			}
			if it.Status != model.CareerPipelineOpen || strings.TrimSpace(it.ListingURL) == "" {
				continue
			}
			toScore = append(toScore, it.ListingURL)
		}
	}

	out := &model.CareerBatchResult{Results: []model.CareerEvaluateResult{}}
	seen := map[string]bool{}
	for _, listingURL := range toScore {
		if seen[listingURL] {
			out.Skipped++
			continue
		}
		seen[listingURL] = true
		res, err := s.Evaluate(ctx, orgID, userID, model.EvaluateCareerRequest{JobURL: listingURL, Mode: mode})
		if err != nil {
			out.Skipped++
			s.logger.Warn("career: batch evaluate skipped", zap.String("url", listingURL), zap.Error(err))
			continue
		}
		if res.BlacklistHit != nil || res.Dead || res.Evaluation == nil {
			out.Skipped++
			continue
		}
		out.Evaluated++
		out.Results = append(out.Results, *res)
	}
	return out, nil
}

var _ CareerService = (*careerService)(nil)
