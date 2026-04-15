package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	miniogo "github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/rs/cors"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/bridge"
	"github.com/jobshout/server/internal/config"
	"github.com/jobshout/server/internal/costengine"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	integ "github.com/jobshout/server/internal/integration"
	emailAdapter "github.com/jobshout/server/internal/integration/adapters/email"
	githubAdapter "github.com/jobshout/server/internal/integration/adapters/github"
	jiraAdapter "github.com/jobshout/server/internal/integration/adapters/jira"
	slackAdapter "github.com/jobshout/server/internal/integration/adapters/slack"
	teamsAdapter "github.com/jobshout/server/internal/integration/adapters/teams"
	telegramBot "github.com/jobshout/server/internal/integration/adapters/telegram"
	"github.com/jobshout/server/internal/database"
	"github.com/jobshout/server/internal/engine"
	"github.com/jobshout/server/internal/executor"
	"github.com/jobshout/server/internal/handler"
	"github.com/jobshout/server/internal/langchain"
	"github.com/jobshout/server/internal/langgraph"
	"github.com/jobshout/server/internal/llm"
	"github.com/jobshout/server/internal/middleware"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
	"github.com/jobshout/server/internal/selector"
	"github.com/jobshout/server/internal/service"
	"github.com/jobshout/server/internal/tools"
	ws "github.com/jobshout/server/internal/websocket"
	wfengine "github.com/jobshout/server/internal/workflow"

	"github.com/google/uuid"
)

const version = "0.3.0"

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	cfg, err := config.Load()
	if err != nil {
		logger.Fatal("failed to load config", zap.Error(err))
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool, err := database.NewPool(ctx, cfg.DatabaseURL, logger)
	if err != nil {
		logger.Fatal("failed to connect to database", zap.Error(err))
	}
	defer pool.Close()

	// Run migrations
	if err := database.RunMigrations(ctx, pool, "migrations", logger); err != nil {
		logger.Fatal("failed to run migrations", zap.Error(err))
	}

	// ─── Repositories ────────────────────────────────────────────────────────
	userRepo := repository.NewUserRepository(pool)
	tokenRepo := repository.NewTokenRepository(pool)
	orgRepo := repository.NewOrganizationRepository(pool)
	agentRepo := repository.NewAgentRepository(pool)
	projectRepo := repository.NewProjectRepository(pool)
	taskRepo := repository.NewTaskRepository(pool)
	workflowRepo := repository.NewWorkflowRepository(pool)
	execRepo := repository.NewExecutionRepository(pool)
	toolPermRepo := repository.NewAgentToolRepository(pool)
	llmProviderRepo := repository.NewLLMProviderRepository(pool)
	schedulerRepo := repository.NewSchedulerRepository(pool)
	sessionRepo := repository.NewSessionRepository(pool)
	pluginRepo := repository.NewPluginRepository(pool)
	integRepo := repository.NewIntegrationRepository(pool)
	linkRepo := repository.NewTaskLinkRepository(pool)
	syncLogRepo := repository.NewSyncLogRepository(pool)
	notifConfigRepo := repository.NewNotificationConfigRepository(pool)
	usageRepo := repository.NewUsageRepository(pool)
	budgetRepo := repository.NewBudgetRepository(pool)
	policyRepo := repository.NewPolicyRepository(pool)
	rbacRepo := repository.NewRBACRepository(pool)
	ssoRepo := repository.NewSSORepository(pool)
	auditRepo := repository.NewAuditRepository(pool)
	pricingRepo := repository.NewPricingRepository(pool)

	// Autonomous agents + chat + Telegram repositories
	memoryRepo := repository.NewMemoryRepository(pool)
	goalRepo := repository.NewGoalRepository(pool)
	multiAgentRepo := repository.NewMultiAgentRepository(pool)
	chatRepo := repository.NewChatRepository(pool)
	telegramRepo := repository.NewTelegramRepository(pool)

	// ─── LLM layer ───────────────────────────────────────────────────────────
	// Ollama running locally is the default; OpenAI is used when configured.
	llmRouter := llm.NewRouter(cfg)
	logger.Info("LLM router initialised",
		zap.String("default_provider", cfg.LLMProvider),
		zap.String("ollama_url", cfg.OllamaBaseURL),
		zap.String("ollama_model", cfg.OllamaDefaultModel),
	)

	// ─── Tool registry ───────────────────────────────────────────────────────
	toolRegistry := tools.NewRegistry()
	toolRegistry.Register(tools.NewHTTPTool())
	toolRegistry.Register(tools.NewShellTool(nil))
	logger.Info("tool registry initialised", zap.Int("tools", len(toolRegistry.All())))

	// ─── Engine Router (multi-runtime) ──────────────────────────────────────
	goNativeExec := executor.New(llmRouter, toolRegistry, logger)

	// Python sidecar clients for LangChain/LangGraph (nil-safe if not configured).
	var lcClient *langchain.Client
	var lgClient *langgraph.Client
	if cfg.PythonSidecarURL != "" {
		lcClient = langchain.NewClient(cfg.PythonSidecarURL, cfg.PythonSidecarSecret, logger)
		lgClient = langgraph.NewClient(cfg.PythonSidecarURL, cfg.PythonSidecarSecret, logger)
		logger.Info("Python sidecar clients initialised",
			zap.String("sidecar_url", cfg.PythonSidecarURL),
		)
	}

	var lcRunner engine.Runner
	var lgRunner engine.Runner
	if lcClient != nil {
		lcRunner = lcClient
	}
	if lgClient != nil {
		lgRunner = lgClient
	}
	engineRouter := engine.NewRouter(goNativeExec, lcRunner, lgRunner, logger)

	// ─── Workflow DAG engine ─────────────────────────────────────────────────
	agentResolver := func(ctx context.Context, agentID uuid.UUID) (*model.Agent, error) {
		return agentRepo.FindByID(ctx, agentID)
	}
	toolPermResolver := func(ctx context.Context, agentID uuid.UUID) ([]string, error) {
		return toolPermRepo.ListByAgent(ctx, agentID)
	}
	dagPersister := service.NewDagPersister(execRepo)

	dagEngine := wfengine.NewEngine(
		engineRouter,
		agentResolver,
		toolPermResolver,
		dagPersister,
		logger,
	)

	// ─── Cost Engine ─────────────────────────────────────────────────────────
	costEng := costengine.New()
	logger.Info("cost engine initialised", zap.Int("known_models", len(costEng.KnownModels())))

	// ─── Services ────────────────────────────────────────────────────────────
	jwtSvc := service.NewJWTService(cfg)
	authSvc := service.NewAuthService(userRepo, tokenRepo, orgRepo, jwtSvc, logger)
	agentSvc := service.NewAgentService(agentRepo, logger)
	projectSvc := service.NewProjectService(projectRepo, logger)
	taskSvc := service.NewTaskService(taskRepo, logger)
	govSvc := service.NewGovernanceService(budgetRepo, policyRepo, usageRepo, execRepo, costEng, logger)
	analyticsSvc := service.NewAnalyticsService(usageRepo, logger)
	rbacSvc := service.NewRBACService(rbacRepo, logger)
	ssoSvc := service.NewSSOService(ssoRepo, userRepo, rbacRepo, auditRepo, logger)
	leaderboardSvc := service.NewLeaderboardService(usageRepo, logger)
	execSvc := service.NewExecutionService(agentRepo, execRepo, toolPermRepo, engineRouter, govSvc, logger)
	workflowSvc := service.NewWorkflowService(workflowRepo, agentRepo, execRepo, toolPermRepo, dagEngine, logger)
	pluginSvc := service.NewPluginService(pluginRepo, agentRepo, engineRouter, logger)

	// ─── Autonomous agent engine ────────────────────────────────────────────
	autonomousExec := executor.NewAutonomousExecutor(goNativeExec, llmRouter, memoryRepo, goalRepo, logger)
	memorySvc := service.NewMemoryService(memoryRepo, logger)
	intentSvc := service.NewIntentService(llmRouter, logger)
	goalSvc := service.NewGoalService(goalRepo, agentRepo, toolPermRepo, autonomousExec, logger)
	multiAgentSvc := service.NewMultiAgentService(multiAgentRepo, agentRepo, toolPermRepo, autonomousExec, logger)
	chatSvc := service.NewChatService(chatRepo, intentSvc, memorySvc, goalSvc, logger)
	_ = memorySvc // used by chatSvc

	// ─── Telegram bot (conditional on config) ───────────────────────────────
	var telegramSvc service.TelegramService
	var tgBot *telegramBot.BotClient
	if cfg.TelegramBotToken != "" {
		tgBot = telegramBot.NewBotClient(cfg.TelegramBotToken)
		telegramSvc = service.NewTelegramService(
			tgBot, telegramRepo, chatSvc,
			cfg.TelegramRatePerMin, cfg.FrontendBaseURL, logger,
		)
		// Register webhook at startup.
		if cfg.TelegramWebhookURL != "" {
			go func() {
				if err := tgBot.SetWebhook(ctx, cfg.TelegramWebhookURL, cfg.TelegramSecretToken); err != nil {
					logger.Warn("failed to register telegram webhook", zap.Error(err))
				} else {
					logger.Info("telegram webhook registered", zap.String("url", cfg.TelegramWebhookURL))
				}
			}()
		}
		logger.Info("Telegram bot initialised")
	}

	// ─── Integration framework ──────────────────────────────────────────────
	adapterRegistry := integ.NewRegistry()
	adapterRegistry.RegisterTask("jira", jiraAdapter.NewAdapter)
	adapterRegistry.RegisterTask("github", githubAdapter.NewAdapter)
	adapterRegistry.RegisterNotification("slack", slackAdapter.NewAdapter)
	adapterRegistry.RegisterNotification("teams", teamsAdapter.NewAdapter)
	adapterRegistry.RegisterNotification("email", emailAdapter.NewAdapter)

	eventBus := integ.NewBus()
	integSvc := service.NewIntegrationService(integRepo, linkRepo, syncLogRepo, adapterRegistry, logger)
	notifSvc := service.NewNotificationService(notifConfigRepo, adapterRegistry, logger)
	budgetAlertDispatcher := service.NewBudgetAlertDispatcher(notifSvc, logger)
	_ = budgetAlertDispatcher // available for governance service to dispatch budget alerts
	go notifSvc.StartSubscriber(ctx, eventBus)
	logger.Info("integration framework initialised",
		zap.Int("task_adapters", 2),
		zap.Int("notification_adapters", 3),
	)

	// ─── Bridge client (SSE streaming) ──────────────────────────────────────
	var bridgeClient *bridge.Client
	if cfg.PythonSidecarURL != "" {
		bridgeClient = bridge.NewClient(cfg.PythonSidecarURL, cfg.PythonSidecarSecret, logger)
	}

	// ─── WebSocket hub ───────────────────────────────────────────────────────
	hub := ws.NewHub(logger)
	go hub.Run()

	// ─── MinIO client (optional) ─────────────────────────────────────────────
	var uploadHandler *handler.UploadHandler
	if cfg.MinIOEndpoint != "" {
		minioClient, err := miniogo.New(cfg.MinIOEndpoint, &miniogo.Options{
			Creds:  credentials.NewStaticV4(cfg.MinIOAccessKey, cfg.MinIOSecretKey, ""),
			Secure: cfg.MinIOUseSSL,
		})
		if err != nil {
			logger.Warn("failed to create minio client — uploads disabled", zap.Error(err))
		} else {
			uploadHandler = handler.NewUploadHandler(minioClient, cfg.MinIOBucketAvatars, logger)
		}
	}

	// ─── Handlers ────────────────────────────────────────────────────────────
	authHandler := handler.NewAuthHandler(authSvc)
	agentHandler := handler.NewAgentHandler(agentSvc)
	projectHandler := handler.NewProjectHandler(projectSvc)
	taskHandler := handler.NewTaskHandler(taskSvc)
	orgHandler := handler.NewOrganizationHandler(orgRepo)
	marketplaceHandler := handler.NewMarketplaceHandler(pool, logger)
	knowledgeHandler := handler.NewKnowledgeHandler(pool, logger)
	metricsHandler := handler.NewMetricsHandler(pool, logger)
	wsHandler := handler.NewWSHandler(hub, logger)
	execHandler := handler.NewExecutionHandler(execSvc)
	workflowHandler := handler.NewWorkflowHandler(workflowSvc)
	engineHandler := handler.NewEngineHandler(lcClient, lgClient, logger)
	pluginHandler := handler.NewPluginHandler(pluginSvc)
	streamHandler := handler.NewStreamHandler(bridgeClient, logger)
	llmProviderHandler := handler.NewLLMProviderHandler(llmProviderRepo, llmRouter)
	schedulerHandler := handler.NewSchedulerHandler(schedulerRepo)
	sessionHandler := handler.NewSessionHandler(sessionRepo)
	integHandler := handler.NewIntegrationHandler(integSvc)
	notifHandler := handler.NewNotificationHandler(notifSvc)
	webhookHandler := handler.NewWebhookHandler(integRepo, linkRepo, logger)
	governanceHandler := handler.NewGovernanceHandler(govSvc)
	analyticsHandler := handler.NewAnalyticsHandler(analyticsSvc)
	rbacHandler := handler.NewRBACHandler(rbacSvc)
	ssoHandler := handler.NewSSOHandler(ssoSvc, jwtSvc)
	auditHandler := handler.NewAuditHandler(auditRepo)
	pricingHandler := handler.NewPricingHandler(pricingRepo)
	leaderboardHandler := handler.NewLeaderboardHandler(leaderboardSvc)

	// Chat, goal, multi-agent, and Telegram handlers
	chatHandler := handler.NewChatHandler(chatSvc)
	goalHandler := handler.NewGoalHandler(goalSvc)
	multiAgentHandler := handler.NewMultiAgentHandler(multiAgentSvc)
	var telegramHandler *handler.TelegramHandler
	if telegramSvc != nil {
		telegramHandler = handler.NewTelegramHandler(telegramSvc, cfg.TelegramSecretToken, logger)
	}

	// Agent selector
	agentSelector := selector.New(pool, logger)
	selectorHandler := handler.NewSelectorHandler(agentSelector)

	// Auth middleware
	requireAuth := middleware.RequireAuth(jwtSvc)

	r := chi.NewRouter()

	// Global middleware
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(chimiddleware.Recoverer)
	r.Use(chimiddleware.Timeout(30 * time.Second))

	// CORS
	corsHandler := cors.New(cors.Options{
		AllowedOrigins:   cfg.CORSOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Request-ID"},
		ExposedHeaders:   []string{"X-Request-ID"},
		AllowCredentials: true,
		MaxAge:           300,
	})
	r.Use(corsHandler.Handler)

	// Health check
	r.Get("/health", handler.Health(pool, version))

	// Prometheus metrics endpoint
	r.Handle("/metrics", promhttp.Handler())

	// Public webhook endpoints (no auth — verified by HMAC/secret token)
	r.Route("/webhooks", func(r chi.Router) {
		r.Post("/jira/{integrationID}", webhookHandler.Jira)
		r.Post("/github/{integrationID}", webhookHandler.GitHub)
		if telegramHandler != nil {
			r.Post("/telegram", telegramHandler.Webhook)
		}
	})

	// API v1 routes
	r.Route("/api/v1", func(r chi.Router) {
		// Public auth routes
		r.Post("/auth/register", authHandler.Register)
		r.Post("/auth/login", authHandler.Login)
		r.Post("/auth/refresh", authHandler.Refresh)

		// Protected routes
		r.Group(func(r chi.Router) {
			r.Use(requireAuth)

			r.Get("/auth/me", authHandler.GetMe)
			r.Patch("/auth/me", authHandler.UpdateProfile)

			// Agents
			r.Route("/agents", func(r chi.Router) {
				r.Get("/", agentHandler.List)
				r.Post("/", agentHandler.Create)
				r.Route("/{agentID}", func(r chi.Router) {
					r.Get("/", agentHandler.GetByID)
					r.Put("/", agentHandler.Update)
					r.Delete("/", agentHandler.Delete)
					r.Patch("/status", agentHandler.UpdateStatus)

					// Agent LLM execution
					r.Post("/execute", execHandler.Execute)
					r.Get("/executions", execHandler.ListByAgent)

					// Autonomous agent goals
					r.Route("/goals", func(r chi.Router) {
						r.Get("/", goalHandler.ListGoals)
						r.Post("/", goalHandler.CreateGoal)
					})
				})
			})

			// Goal lookup by ID
			r.Get("/goals/{goalID}", goalHandler.GetGoal)

			// Agent execution lookup by ID (standalone) + trace endpoints
			r.Get("/executions/{executionID}", execHandler.GetExecution)
			r.Get("/executions/{executionID}/langchain-traces", execHandler.ListLangChainTraces)
			r.Get("/executions/{executionID}/langgraph-snapshots", execHandler.ListLangGraphSnapshots)

			// Execution engines
			r.Get("/engines", engineHandler.List)
			r.Get("/engines/health", engineHandler.Health)

			// Projects
			r.Route("/projects", func(r chi.Router) {
				r.Get("/", projectHandler.List)
				r.Post("/", projectHandler.Create)
				r.Route("/{projectID}", func(r chi.Router) {
					r.Get("/", projectHandler.GetByID)
					r.Put("/", projectHandler.Update)
					r.Delete("/", projectHandler.Delete)
					// Nested tasks route: rewrites project_id from URL path to query param
					r.Get("/tasks", func(w http.ResponseWriter, r *http.Request) {
						projectID := chi.URLParam(r, "projectID")
						q := r.URL.Query()
						q.Set("project_id", projectID)
						r.URL.RawQuery = q.Encode()
						taskHandler.List(w, r)
					})
				})
			})

			// Tasks
			r.Route("/tasks", func(r chi.Router) {
				r.Get("/", taskHandler.List)
				r.Post("/", taskHandler.Create)
				r.Route("/{taskID}", func(r chi.Router) {
					r.Get("/", taskHandler.GetByID)
					r.Put("/", taskHandler.Update)
					r.Delete("/", taskHandler.Delete)
					r.Patch("/transition", taskHandler.Transition)
					r.Put("/position", taskHandler.Reorder)
					r.Get("/comments", taskHandler.ListComments)
					r.Post("/comments", taskHandler.AddComment)
				})
			})

			// Organizations
			r.Route("/organizations/{orgID}", func(r chi.Router) {
				r.Get("/", orgHandler.GetByID)
				r.Put("/", orgHandler.Update)
				r.Put("/chart", orgHandler.UpdateChart)
			})

			// Knowledge files (nested under agents)
			r.Route("/agents/{agentID}/knowledge", func(r chi.Router) {
				r.Get("/", knowledgeHandler.ListByAgent)
				r.Post("/", knowledgeHandler.CreateFile)
				r.Route("/{fileID}", func(r chi.Router) {
					r.Get("/", knowledgeHandler.GetFile)
					r.Put("/", knowledgeHandler.UpdateFile)
					r.Delete("/", knowledgeHandler.DeleteFile)
				})
			})

			// Marketplace
			r.Route("/marketplace", func(r chi.Router) {
				r.Get("/", marketplaceHandler.List)
				r.Route("/{agentID}", func(r chi.Router) {
					r.Get("/", marketplaceHandler.GetByID)
					r.Post("/import", marketplaceHandler.Import)
				})
			})

			// Metrics
			r.Route("/metrics", func(r chi.Router) {
				r.Get("/summary", metricsHandler.Summary)
				r.Get("/agents/{agentID}", metricsHandler.AgentMetrics)
				r.Get("/task-completion", metricsHandler.TaskCompletion)
			})

			// Workflows
			r.Route("/workflows", func(r chi.Router) {
				r.Get("/", workflowHandler.List)
				r.Post("/", workflowHandler.Create)
				r.Route("/{workflowID}", func(r chi.Router) {
					r.Get("/", workflowHandler.GetByID)
					r.Put("/", workflowHandler.Update)
					r.Delete("/", workflowHandler.Delete)
					r.Post("/execute", workflowHandler.ExecuteWorkflow)
					r.Get("/runs", workflowHandler.ListRuns)
				})
			})

			// Workflow run status polling
			r.Get("/workflow-runs/{runID}", workflowHandler.GetRun)

			// Plugins (user-defined LangGraph/LangChain workflows)
			r.Route("/plugins", func(r chi.Router) {
				r.Get("/", pluginHandler.List)
				r.Post("/", pluginHandler.Create)
				r.Route("/{pluginID}", func(r chi.Router) {
					r.Get("/", pluginHandler.GetByID)
					r.Put("/", pluginHandler.Update)
					r.Delete("/", pluginHandler.Delete)
					r.Post("/execute", pluginHandler.Execute)
					r.Get("/executions", pluginHandler.ListExecutions)
				})
			})

			// SSE Streaming execution
			r.Post("/stream/execute", streamHandler.StreamExecute)
			r.Get("/workflows/{workflowID}/stream/{stepName}", streamHandler.StreamWorkflowStep)

			// LLM Provider Configs
			r.Route("/llm-providers", func(r chi.Router) {
				r.Get("/builtin", llmProviderHandler.ListBuiltin)
				r.Get("/", llmProviderHandler.List)
				r.Post("/", llmProviderHandler.Create)
				r.Route("/{providerID}", func(r chi.Router) {
					r.Get("/", llmProviderHandler.GetByID)
					r.Put("/", llmProviderHandler.Update)
					r.Delete("/", llmProviderHandler.Delete)
				})
			})

			// Scheduled Tasks
			r.Route("/scheduled-tasks", func(r chi.Router) {
				r.Get("/", schedulerHandler.List)
				r.Post("/", schedulerHandler.Create)
				r.Route("/{taskID}", func(r chi.Router) {
					r.Get("/", schedulerHandler.GetByID)
					r.Put("/", schedulerHandler.Update)
					r.Delete("/", schedulerHandler.Delete)
					r.Get("/runs", schedulerHandler.ListRuns)
				})
			})

			// Sessions (context management across LLM switches)
			r.Route("/sessions", func(r chi.Router) {
				r.Get("/", sessionHandler.List)
				r.Post("/", sessionHandler.Create)
				r.Route("/{sessionID}", func(r chi.Router) {
					r.Get("/", sessionHandler.GetByID)
					r.Put("/", sessionHandler.Update)
					r.Delete("/", sessionHandler.Delete)
					r.Post("/copy-context", sessionHandler.CopyContext)
					r.Post("/snapshots", sessionHandler.CreateSnapshot)
					r.Get("/snapshots", sessionHandler.ListSnapshots)
					r.Post("/snapshots/{snapshotID}/restore", sessionHandler.RestoreSnapshot)
				})
			})

			// Integrations (Jira, GitHub)
			r.Route("/integrations", func(r chi.Router) {
				r.Get("/", integHandler.List)
				r.Post("/", integHandler.Create)
				r.Route("/{integrationID}", func(r chi.Router) {
					r.Get("/", integHandler.Get)
					r.Put("/", integHandler.Update)
					r.Delete("/", integHandler.Delete)
					r.Get("/links", integHandler.ListLinks)
					r.Get("/sync-logs", integHandler.ListSyncLogs)
					r.Post("/tasks/{taskID}/link", integHandler.LinkTask)
					r.Delete("/tasks/{taskID}/link", integHandler.UnlinkTask)
					r.Post("/links/{linkID}/sync", integHandler.SyncLink)
				})
			})

			// Notifications (Slack, Teams)
			r.Route("/notifications", func(r chi.Router) {
				r.Get("/", notifHandler.List)
				r.Post("/", notifHandler.Create)
				r.Route("/{configID}", func(r chi.Router) {
					r.Get("/", notifHandler.Get)
					r.Put("/", notifHandler.Update)
					r.Delete("/", notifHandler.Delete)
					r.Post("/test", notifHandler.Test)
				})
			})

			// Governance (budgets + policies)
			r.Route("/governance", func(r chi.Router) {
				r.Get("/budgets", governanceHandler.ListBudgets)
				r.Post("/budgets", governanceHandler.UpsertBudget)
				r.Delete("/budgets/{budgetID}", governanceHandler.DeleteBudget)
				r.Get("/budgets/alerts", governanceHandler.ListAlerts)
				r.Get("/policies", governanceHandler.ListPolicies)
				r.Post("/policies", governanceHandler.UpsertPolicy)
				r.Delete("/policies/{policyID}", governanceHandler.DeletePolicy)
			})

			// Analytics (usage, costs, top agents)
			r.Route("/analytics", func(r chi.Router) {
				r.Get("/usage", analyticsHandler.UsageTimeSeries)
				r.Get("/usage/summary", analyticsHandler.OrgUsageSummary)
				r.Get("/agents/{agentID}", analyticsHandler.AgentAnalytics)
				r.Get("/top-agents", analyticsHandler.TopAgents)
			})

			// RBAC (roles and permissions)
			r.Route("/rbac", func(r chi.Router) {
				r.Get("/me/permissions", rbacHandler.MyPermissions)
				r.Get("/roles", rbacHandler.ListRoles)
				r.Post("/roles", rbacHandler.CreateRole)
				r.Delete("/roles/{roleID}", rbacHandler.DeleteRole)
				r.Post("/assignments", rbacHandler.AssignRole)
				r.Delete("/assignments/{userID}/{roleID}", rbacHandler.RemoveRole)
				r.Get("/users/{userID}/roles", rbacHandler.ListUserRoles)
			})

			// SSO (OIDC config + login flows)
			r.Route("/sso", func(r chi.Router) {
				r.Get("/configs", ssoHandler.ListConfigs)
				r.Post("/configs", ssoHandler.CreateConfig)
				r.Delete("/configs/{configID}", ssoHandler.DeleteConfig)
				r.Get("/authorize", ssoHandler.Authorize)
				r.Post("/callback", ssoHandler.Callback)
				r.Get("/login-audit", ssoHandler.ListLoginAudit)
			})

			// Audit logs
			r.Route("/audit", func(r chi.Router) {
				r.Get("/actions", auditHandler.ListActions)
				r.Get("/logins", auditHandler.ListLogins)
			})

			// Pricing configuration
			r.Route("/pricing", func(r chi.Router) {
				r.Get("/", pricingHandler.ListActive)
				r.Post("/", pricingHandler.Create)
				r.Delete("/{configID}", pricingHandler.Deactivate)
			})

			// Agent leaderboard + anomaly detection
			r.Route("/leaderboard", func(r chi.Router) {
				r.Get("/", leaderboardHandler.Leaderboard)
				r.Get("/anomalies", leaderboardHandler.Anomalies)
			})

			// Chat sessions
			r.Route("/chat/sessions", func(r chi.Router) {
				r.Get("/", chatHandler.ListSessions)
				r.Post("/", chatHandler.StartSession)
				r.Route("/{sessionID}", func(r chi.Router) {
					r.Get("/messages", chatHandler.GetHistory)
					r.Post("/messages", chatHandler.SendMessage)
				})
			})

			// Multi-agent collaboration jobs
			r.Route("/multi-agent/jobs", func(r chi.Router) {
				r.Get("/", multiAgentHandler.ListJobs)
				r.Post("/", multiAgentHandler.RunJob)
				r.Get("/{jobID}", multiAgentHandler.GetJob)
			})

			// Telegram account management
			if telegramHandler != nil {
				r.Route("/telegram", func(r chi.Router) {
					r.Post("/link-token", telegramHandler.GenerateLinkToken)
					r.Delete("/unlink", telegramHandler.UnlinkUser)
					r.Get("/status", telegramHandler.LinkStatus)
				})
			}

			// Cost-aware agent selection
			r.Post("/agents/select", selectorHandler.Select)
			r.Post("/agents/scores/refresh", selectorHandler.RefreshScores)

			// Uploads (MinIO)
			if uploadHandler != nil {
				r.Post("/uploads/avatar", uploadHandler.UploadAvatar)
				r.Get("/uploads/avatar/*", uploadHandler.ServeAvatar)
			}

			// WebSocket
			r.Get("/ws", wsHandler.Connect)
		})
	})

	srv := &http.Server{
		Addr:         cfg.ServerPort,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 120 * time.Second, // increased for LLM calls
		IdleTimeout:  60 * time.Second,
	}

	// Start server in a goroutine
	go func() {
		logger.Info("starting server",
			zap.String("port", cfg.ServerPort),
			zap.String("version", version),
		)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("server failed", zap.Error(err))
		}
	}()

	// Graceful shutdown on SIGINT/SIGTERM
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down server...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Fatal("server forced shutdown", zap.Error(err))
	}

	logger.Info("server stopped")
}
