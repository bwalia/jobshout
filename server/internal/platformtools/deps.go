package platformtools

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jobshout/server/internal/agentmodule"
	"github.com/jobshout/server/internal/repository"
	"github.com/jobshout/server/internal/service"
	"github.com/jobshout/server/internal/tasklaunch"
	"github.com/jobshout/server/internal/tools"
)

// Deps is the service bundle platform tools call into. Fields may be nil;
// the corresponding tools then return a clear "not configured" result.
type Deps struct {
	Agents          service.AgentService
	Exec            service.ExecutionService
	Workflows       service.WorkflowService
	Tasks           service.TaskService
	Projects        service.ProjectService
	Goals           service.GoalService
	Research        service.ResearchService
	Blog            service.BlogService
	Pentest         service.PentestService
	Images          *service.ImageService
	Reviews         service.ReviewService
	Mail            service.MailService
	Career          service.CareerService
	MultiAgent      service.MultiAgentService
	Sprints         service.SprintService
	Plugins         service.PluginService
	MCP             service.MCPService
	Integrations    service.IntegrationService
	Notifications   service.NotificationService
	Approvals       service.ApprovalService
	Analytics       service.AnalyticsService
	Leaderboard     service.LeaderboardService
	Governance      service.GovernanceService
	RBAC            service.RBACService
	Scheduler       repository.SchedulerRepository
	Skills          repository.SkillRepository
	LLMProviders    repository.LLMProviderRepository
	Audit           repository.AuditRepository
	Sessions        repository.SessionRepository
	Knowledge       service.KnowledgeIngestService
	KnowledgeSearch tools.KnowledgeSearcher
	Embedder        tools.Embedder
	Pool            *pgxpool.Pool
	Memory          service.MemoryService
	Launch          *tasklaunch.Service
}

// NewRegistryWithTools builds the chat platform registry. Nil deps fields
// skip the tools that need them rather than panicking.
func NewRegistryWithTools(d Deps) *Registry {
	reg := NewRegistry()
	registerHelp(reg)
	registerRemember(reg, d)
	if d.Agents != nil {
		registerAgents(reg, d)
	}
	if d.Tasks != nil && d.Projects != nil {
		registerWork(reg, d)
	}
	if d.Workflows != nil {
		registerWorkflows(reg, d)
	}
	// Extra chat tools attach from the specialist registry (InstallTools on the
	// module, or SetToolInstaller from the agent's tools file).
	// All specialists are wired this way; a new agent does not need a line here —
	// register it, do not add a switch.
	for _, m := range agentmodule.All() {
		fn := m.InstallTools
		if fn == nil {
			fn = agentmodule.ToolInstaller(m.Builtin)
		}
		if fn != nil {
			fn(reg, d)
		}
	}
	registerConfig(reg, d)
	registerInsight(reg, d)
	registerSecurity(reg, d)
	registerCatalog(reg)
	return reg
}

func wrapInstall(fn func(*Registry, Deps)) func(reg, deps any) {
	return func(reg, deps any) {
		r, ok1 := reg.(*Registry)
		d, ok2 := deps.(Deps)
		if ok1 && ok2 {
			fn(r, d)
		}
	}
}
