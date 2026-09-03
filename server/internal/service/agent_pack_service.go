package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/agentmodule"
	"github.com/jobshout/server/internal/agentpack"
	"github.com/jobshout/server/internal/llm"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
	"github.com/jobshout/server/internal/tools"
)

const previewTTL = 15 * time.Minute

// builtinFinder is the slice of AgentRepository import/export needs.
type builtinFinder interface {
	FindBuiltin(ctx context.Context, orgID uuid.UUID, builtin string) (*model.Agent, error)
}

type AgentPackService interface {
	Export(ctx context.Context, orgID, userID, agentID uuid.UUID) (*agentpack.Package, string, error)
	Preview(ctx context.Context, orgID uuid.UUID, pkg *agentpack.Package) (*PackPreview, error)
	ResolvePreview(ctx context.Context, orgID uuid.UUID, req ImportAgentRequest) (agentpack.Report, error)
	Import(ctx context.Context, orgID, userID uuid.UUID, req ImportAgentRequest) (*ImportAgentResult, error)
}

type PackPreview struct {
	PreviewID string `json:"preview_id"`
	agentpack.Report
}

type ImportAgentRequest struct {
	PreviewID string             `json:"preview_id"`
	Package   *agentpack.Package `json:"package"`
	Bindings  agentpack.Bindings `json:"bindings"`
}

type ImportAgentResult struct {
	Agent   *model.Agent   `json:"agent"`
	Mode    agentpack.Mode `json:"mode"`
	CanUndo bool           `json:"can_undo"`
}

type cachedPack struct {
	pkg     *agentpack.Package
	orgID   uuid.UUID
	expires time.Time
}

type agentPackService struct {
	store     repository.AgentPackStore
	agents    builtinFinder
	skills    repository.SkillRepository
	tools     *tools.Registry
	models    *llm.Router
	ingest    KnowledgeIngestService
	audit     repository.AuditRepository
	autoModel bool
	logger    *zap.Logger

	mu    sync.Mutex
	cache map[string]cachedPack
}

func NewAgentPackService(
	store repository.AgentPackStore,
	agents builtinFinder,
	skills repository.SkillRepository,
	toolReg *tools.Registry,
	models *llm.Router,
	ingest KnowledgeIngestService,
	audit repository.AuditRepository,
	autoModel bool,
	logger *zap.Logger,
) AgentPackService {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &agentPackService{
		store:     store,
		agents:    agents,
		skills:    skills,
		tools:     toolReg,
		models:    models,
		ingest:    ingest,
		audit:     audit,
		autoModel: autoModel,
		logger:    logger,
		cache:     map[string]cachedPack{},
	}
}

func (s *agentPackService) Export(ctx context.Context, orgID, userID, agentID uuid.UUID) (*agentpack.Package, string, error) {
	b, err := s.store.LoadBundle(ctx, orgID, agentID)
	if err != nil {
		return nil, "", err
	}
	pkg, err := agentpack.Pack(agentpack.Input{
		Agent:     b.Agent,
		Tools:     b.Tools,
		Skills:    b.Skills,
		Knowledge: b.Knowledge,
	})
	if err != nil {
		return nil, "", err
	}
	var who *uuid.UUID
	if userID != uuid.Nil {
		who = &userID
	}
	s.record(ctx, orgID, who, "agent.export", &agentID, map[string]any{
		"builtin": pkg.Agent.Builtin, "warnings": len(pkg.Warnings),
	})
	return pkg, agentpack.FilenameSlug(pkg.Agent.Name, pkg.ExportedAt), nil
}

func (s *agentPackService) Preview(ctx context.Context, orgID uuid.UUID, pkg *agentpack.Package) (*PackPreview, error) {
	if pkg == nil {
		return nil, fmt.Errorf("package is required")
	}
	agentpack.SanitizePackage(pkg)
	if err := agentpack.ValidateKind(pkg); err != nil {
		return nil, err
	}
	if err := agentpack.CheckSize(pkg); err != nil {
		return nil, err
	}
	dest, err := s.dest(ctx, orgID, pkg)
	if err != nil {
		return nil, err
	}
	rep := agentpack.Evaluate(pkg, dest)
	id := uuid.New().String()
	s.mu.Lock()
	s.pruneLocked(time.Now())
	s.cache[id] = cachedPack{pkg: clonePkg(pkg), orgID: orgID, expires: time.Now().Add(previewTTL)}
	s.mu.Unlock()
	return &PackPreview{PreviewID: id, Report: rep}, nil
}

func (s *agentPackService) Import(ctx context.Context, orgID, userID uuid.UUID, req ImportAgentRequest) (*ImportAgentResult, error) {
	pkg, rep, err := s.resolve(ctx, orgID, req)
	if err != nil {
		return nil, err
	}
	if rep.HasError() {
		return nil, fmt.Errorf("%s", firstError(rep))
	}

	known := s.toolSet()
	tools := agentpack.EffectiveTools(pkg, rep.Bindings, known)
	provider := rep.Bindings.ModelProvider
	modelName := rep.Bindings.ModelName

	var agent *model.Agent
	switch rep.Mode {
	case agentpack.ModeOverlay:
		if rep.TargetAgentID == nil {
			return nil, fmt.Errorf("overlay target missing")
		}
		provider, modelName, tools = s.overlayPreserve(ctx, orgID, *rep.TargetAgentID, pkg, provider, modelName, tools)
		agent, err = s.store.ApplyOverlay(ctx, orgID, *rep.TargetAgentID, pkg, provider, modelName, tools)
	default:
		name, nerr := s.uniqueCreateName(ctx, orgID, pkg, rep.Bindings.Name)
		if nerr != nil {
			return nil, nerr
		}
		agent, err = s.store.ApplyCreate(ctx, orgID, userID, pkg, name, provider, modelName, tools)
	}
	if err != nil {
		return nil, err
	}

	s.ingestKnowledge(ctx, orgID, agent.ID)
	s.record(ctx, orgID, &userID, "agent.import", &agent.ID, map[string]any{
		"mode": string(rep.Mode), "builtin": pkg.Agent.Builtin, "can_undo": rep.CanUndo,
	})
	if req.PreviewID != "" {
		s.mu.Lock()
		delete(s.cache, req.PreviewID)
		s.mu.Unlock()
	}
	return &ImportAgentResult{Agent: agent, Mode: rep.Mode, CanUndo: rep.CanUndo}, nil
}

// ResolvePreview repeats Evaluate for an import request so the handler can
// check create vs update permission before applying.
func (s *agentPackService) ResolvePreview(ctx context.Context, orgID uuid.UUID, req ImportAgentRequest) (agentpack.Report, error) {
	_, rep, err := s.resolve(ctx, orgID, req)
	return rep, err
}

func (s *agentPackService) resolve(ctx context.Context, orgID uuid.UUID, req ImportAgentRequest) (*agentpack.Package, agentpack.Report, error) {
	pkg := req.Package
	if req.PreviewID != "" {
		s.mu.Lock()
		c, ok := s.cache[req.PreviewID]
		s.mu.Unlock()
		if ok && time.Now().Before(c.expires) && c.orgID == orgID {
			pkg = clonePkg(c.pkg)
		} else if req.Package == nil {
			return nil, agentpack.Report{}, fmt.Errorf("preview expired; upload the file again")
		}
		// Cache miss (other replica or restart): fall back to the uploaded package.
	}
	if pkg == nil {
		return nil, agentpack.Report{}, fmt.Errorf("package is required")
	}
	agentpack.SanitizePackage(pkg)
	if err := agentpack.ValidateKind(pkg); err != nil {
		return nil, agentpack.Report{}, err
	}
	if err := agentpack.CheckSize(pkg); err != nil {
		return nil, agentpack.Report{}, err
	}
	dest, err := s.dest(ctx, orgID, pkg)
	if err != nil {
		return nil, agentpack.Report{}, err
	}
	rep := agentpack.Evaluate(pkg, dest)
	if req.PreviewID != "" {
		if req.Bindings.Name != "" {
			rep.Bindings.Name = req.Bindings.Name
		}
		rep.Bindings.ModelProvider = req.Bindings.ModelProvider
		rep.Bindings.ModelName = req.Bindings.ModelName
		rep.Bindings.IncludeGatedTools = req.Bindings.IncludeGatedTools
		if req.Bindings.SkipTools != nil {
			rep.Bindings.SkipTools = req.Bindings.SkipTools
		}
	} else {
		if req.Bindings.Name != "" {
			rep.Bindings.Name = req.Bindings.Name
		}
		if req.Bindings.ModelProvider != "" || req.Bindings.ModelName != "" {
			rep.Bindings.ModelProvider = req.Bindings.ModelProvider
			rep.Bindings.ModelName = req.Bindings.ModelName
		}
		rep.Bindings.IncludeGatedTools = req.Bindings.IncludeGatedTools
		if req.Bindings.SkipTools != nil {
			rep.Bindings.SkipTools = req.Bindings.SkipTools
		}
	}
	return pkg, rep, nil
}

func (s *agentPackService) uniqueCreateName(ctx context.Context, orgID uuid.UUID, pkg *agentpack.Package, preferred string) (string, error) {
	for attempt := 0; attempt < 20; attempt++ {
		name := strings.TrimSpace(preferred)
		if name == "" {
			name = strings.TrimSpace(pkg.Agent.Name)
		}
		if attempt > 0 {
			name = agentpack.UniqueName(pkg, true, attempt)
		}
		taken, err := s.store.NameTaken(ctx, orgID, name, nil)
		if err != nil {
			return "", err
		}
		if !taken {
			return name, nil
		}
	}
	return "", fmt.Errorf("an agent named %q already exists", strings.TrimSpace(pkg.Agent.Name))
}

// overlayPreserve keeps the destination model/tools when the package would
// otherwise blank them (unavailable model, or every packaged tool skipped).
func (s *agentPackService) overlayPreserve(ctx context.Context, orgID, agentID uuid.UUID, pkg *agentpack.Package, provider, modelName string, tools []string) (string, string, []string) {
	needModel := provider == "" && modelName == ""
	needTools := len(tools) == 0 && len(pkg.Tools) > 0
	if !needModel && !needTools {
		return provider, modelName, tools
	}
	b, err := s.store.LoadBundle(ctx, orgID, agentID)
	if err != nil || b == nil || b.Agent == nil {
		return provider, modelName, tools
	}
	if needModel {
		provider = derefStr(b.Agent.ModelProvider)
		modelName = derefStr(b.Agent.ModelName)
	}
	if needTools {
		tools = append([]string(nil), b.Tools...)
	}
	return provider, modelName, tools
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func (s *agentPackService) dest(ctx context.Context, orgID uuid.UUID, pkg *agentpack.Package) (agentpack.Dest, error) {
	d := agentpack.Dest{
		ToolNames:  s.toolSet(),
		SkillSlugs: map[string]bool{},
		ModelOK:    s.modelOK(ctx, pkg.Agent.ModelProvider, pkg.Agent.ModelName),
	}
	taken, err := s.store.NameTaken(ctx, orgID, pkg.Agent.Name, nil)
	if err != nil {
		return d, err
	}
	d.NameTaken = taken
	if s.skills != nil {
		list, err := s.skills.List(ctx, orgID)
		if err != nil {
			return d, err
		}
		for _, sk := range list {
			d.SkillSlugs[sk.Slug] = true
		}
	}
	builtin := strings.TrimSpace(pkg.Agent.Builtin)
	if builtin == "" {
		return d, nil
	}
	_, d.ModuleOK = agentmodule.Lookup(builtin)
	d.DestFieldKeys = agentpack.DestFieldKeys(builtin)
	if s.agents != nil && d.ModuleOK {
		existing, err := s.agents.FindBuiltin(ctx, orgID, builtin)
		if err != nil {
			return d, err
		}
		d.ExistingBuiltin = existing
		if existing != nil {
			b, err := s.store.LoadBundle(ctx, orgID, existing.ID)
			if err == nil {
				d.ExistingTools = b.Tools
			}
		}
	}
	if m, ok := agentmodule.Lookup(builtin); ok && m.Ready != nil {
		d.Ready = m.Ready(ctx, orgID)
	}
	return d, nil
}

func (s *agentPackService) toolSet() map[string]bool {
	out := map[string]bool{}
	if s.tools == nil {
		return out
	}
	for _, n := range s.tools.Names() {
		out[n] = true
	}
	return out
}

func (s *agentPackService) modelOK(ctx context.Context, provider, name string) bool {
	if strings.TrimSpace(provider) == "" && strings.TrimSpace(name) == "" {
		return true
	}
	if strings.EqualFold(provider, "auto") {
		return s.autoModel
	}
	if s.models == nil {
		return true
	}
	for _, g := range s.models.AvailableModels(ctx) {
		if !strings.EqualFold(g.Provider, provider) {
			continue
		}
		if name == "" {
			return true
		}
		for _, m := range g.Models {
			if m.Name == name {
				return true
			}
		}
	}
	return false
}

func (s *agentPackService) ingestKnowledge(ctx context.Context, orgID, agentID uuid.UUID) {
	if s.ingest == nil {
		return
	}
	files, err := s.store.KnowledgeFiles(ctx, agentID)
	if err != nil {
		s.logger.Warn("agent_pack: list knowledge for ingest", zap.Error(err))
		return
	}
	for _, f := range files {
		if err := s.ingest.IngestFile(ctx, orgID, agentID, f.ID, f.Content); err != nil {
			s.logger.Warn("agent_pack: knowledge ingest", zap.Error(err), zap.String("file", f.ID.String()))
		}
	}
}

func (s *agentPackService) record(ctx context.Context, orgID uuid.UUID, userID *uuid.UUID, action string, resourceID *uuid.UUID, meta map[string]any) {
	if s.audit == nil {
		return
	}
	log := &model.AuditLog{
		OrgID:      orgID,
		UserID:     userID,
		Action:     action,
		Resource:   "agent",
		ResourceID: resourceID,
		Metadata:   meta,
	}
	if err := s.audit.RecordAction(ctx, log); err != nil {
		s.logger.Warn("agent_pack: audit", zap.Error(err))
	}
}

func (s *agentPackService) pruneLocked(now time.Time) {
	for id, c := range s.cache {
		if now.After(c.expires) {
			delete(s.cache, id)
		}
	}
}

func clonePkg(pkg *agentpack.Package) *agentpack.Package {
	if pkg == nil {
		return nil
	}
	cp := *pkg
	cp.Tools = append([]string(nil), pkg.Tools...)
	cp.Skills = append([]agentpack.Skill(nil), pkg.Skills...)
	cp.Knowledge = append([]agentpack.File(nil), pkg.Knowledge...)
	cp.Warnings = append([]string(nil), pkg.Warnings...)
	if pkg.Agent.EngineConfig != nil {
		cp.Agent.EngineConfig = agentpack.SanitizeEngineConfig(pkg.Agent.EngineConfig)
	}
	if pkg.Source.FieldKeys != nil {
		cp.Source.FieldKeys = append([]string(nil), pkg.Source.FieldKeys...)
	}
	return &cp
}

func firstError(rep agentpack.Report) string {
	for _, i := range rep.Issues {
		if i.IsError() {
			return i.Message
		}
	}
	return "package cannot be imported"
}
