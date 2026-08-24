package platformtools

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jobshout/server/internal/repository"
	"github.com/jobshout/server/internal/service"
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
	registerSpecialists(reg, d)
	registerReview(reg, d)
	registerConfig(reg, d)
	registerInsight(reg, d)
	registerSecurity(reg, d)
	registerCatalog(reg)
	return reg
}
