package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	miniogo "github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/rs/cors"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/blog"
	"github.com/jobshout/server/internal/bridge"
	"github.com/jobshout/server/internal/chatagent"
	"github.com/jobshout/server/internal/chatsvc"
	"github.com/jobshout/server/internal/config"
	"github.com/jobshout/server/internal/costengine"
	"github.com/jobshout/server/internal/scheduler"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/jobshout/server/internal/database"
	"github.com/jobshout/server/internal/engine"
	"github.com/jobshout/server/internal/executor"
	"github.com/jobshout/server/internal/handler"
	"github.com/jobshout/server/internal/imagegen"
	"github.com/jobshout/server/internal/imagestore"
	integ "github.com/jobshout/server/internal/integration"
	emailAdapter "github.com/jobshout/server/internal/integration/adapters/email"
	githubAdapter "github.com/jobshout/server/internal/integration/adapters/github"
	jiraAdapter "github.com/jobshout/server/internal/integration/adapters/jira"
	"github.com/jobshout/server/internal/integration/adapters/opsapi"
	slackAdapter "github.com/jobshout/server/internal/integration/adapters/slack"
	teamsAdapter "github.com/jobshout/server/internal/integration/adapters/teams"
	telegramBot "github.com/jobshout/server/internal/integration/adapters/telegram"
	"github.com/jobshout/server/internal/langchain"
	"github.com/jobshout/server/internal/langfuse"
	"github.com/jobshout/server/internal/langgraph"
	"github.com/jobshout/server/internal/llm"
	"github.com/jobshout/server/internal/llmtrace"
	"github.com/jobshout/server/internal/mail"
	"github.com/jobshout/server/internal/middleware"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/modelselect"
	"github.com/jobshout/server/internal/platformtools"
	"github.com/jobshout/server/internal/repository"
	"github.com/jobshout/server/internal/research"
	"github.com/jobshout/server/internal/reviewbot"
	"github.com/jobshout/server/internal/selector"
	"github.com/jobshout/server/internal/service"
	"github.com/jobshout/server/internal/strix"
	"github.com/jobshout/server/internal/tasklaunch"
	"github.com/jobshout/server/internal/tools"
	ws "github.com/jobshout/server/internal/websocket"
	wfengine "github.com/jobshout/server/internal/workflow"

	"github.com/google/uuid"
)

// version is the release this binary was built as (CI: -ldflags -X main.version=v1.0.8).
// Runtime APP_VERSION (Helm image.tag) wins so the sidebar shows what is actually deployed.
var version = "dev"

var startedAt = time.Now()

// researchRequestTimeout bounds the synchronous research endpoint.
//
// It is deliberately far above the global request timeout: one research call
// plans searches, retrieves several pages and makes a model call per source,
// which is minutes of legitimate work rather than a stuck request. The
// server's WriteTimeout is raised to match, or the response would be cut off
// even when the handler finished in time.
const researchRequestTimeout = 10 * time.Minute

// imageRequestTimeout bounds a synchronous image generation.
//
// Drawing one 1024x576 cover is around forty seconds of GPU time on a warm
// model and minutes on a cold one, and generation is serialised behind a single
// lock because two of these models do not fit in memory at once — so a request
// may also be queueing behind the one in front of it. Under the default
// timeout every generation failed at thirty seconds with "context deadline
// exceeded", which reads as an unreachable image service rather than a request
// that was cut off while it was working. IMAGE_TIMEOUT still bounds the call
// downstream; this only stops the ceiling being lower than the floor.
const imageRequestTimeout = 30 * time.Minute

// chatRequestTimeout bounds a chat turn while the client is still connected.
// The agent run itself is detached from this context (see chatsvc.SendTurn);
// this only keeps the SSE response open long enough to stream the reply.
const chatRequestTimeout = 10 * time.Minute

// defaultRequestTimeout bounds every route that is not doing something
// legitimately slow. Thirty seconds is generous for a database round trip and
// short enough that a stuck handler is not held open.
const defaultRequestTimeout = 30 * time.Second

// firstNonEmptyStr returns the first non-empty value, for logging what a
// setting actually resolved to rather than what was configured.
func firstNonEmptyStr(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// requestTimeout applies a per-route deadline.
//
// It replaces a single global chi Timeout because that cannot be relaxed for
// one route: chi middleware nests, so a longer Timeout mounted inside a
// shorter one never takes effect — the outer deadline has already been set on
// the context and the inner one cannot extend it.
//
// The symptom that produced this was subtle. Synchronous research would return
// after exactly thirty seconds reporting that it had read several sources and
// extracted nothing from any of them, while each model call logged "context
// deadline exceeded" — which reads as the model being slow, when in fact the
// request context underneath it had been cancelled.
func requestTimeout(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		timeout := defaultRequestTimeout
		// Research answers synchronously: it plans searches, retrieves several
		// pages and makes a model call per source. Minutes of real work, not a
		// stuck request. Trending under the same prefix is only HTTP calls, so
		// it keeps the default.
		if r.Method == http.MethodPost && (strings.HasSuffix(r.URL.Path, "/research") ||
			strings.HasSuffix(r.URL.Path, "/tasks/launch")) {
			timeout = researchRequestTimeout
		}
		// One call to a single GPU, which draws for tens of seconds and may
		// queue behind another generation first. Listing models is only an
		// HTTP call to that service, so it keeps the default.
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/images/generate") {
			timeout = imageRequestTimeout
		}
		if r.Method == http.MethodPost && (strings.HasSuffix(r.URL.Path, "/messages") ||
			strings.HasSuffix(r.URL.Path, "/messages/stream") ||
			strings.HasSuffix(r.URL.Path, "/chat/route")) {
			timeout = chatRequestTimeout
		}

		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	cfg, err := config.Load()
	if err != nil {
		logger.Fatal("failed to load config", zap.Error(err))
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool, err := database.NewPoolWithRetry(ctx, cfg.DatabaseURL, logger, cfg.DatabaseConnectTimeout)
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
	approvalRepo := repository.NewApprovalRepository(pool)
	llmProviderRepo := repository.NewLLMProviderRepository(pool)
	schedulerRepo := repository.NewSchedulerRepository(pool)
	sessionRepo := repository.NewSessionRepository(pool)
	pluginRepo := repository.NewPluginRepository(pool)
	integRepo := repository.NewIntegrationRepository(pool)
	linkRepo := repository.NewTaskLinkRepository(pool)
	syncLogRepo := repository.NewSyncLogRepository(pool)
	notifConfigRepo := repository.NewNotificationConfigRepository(pool)
	mcpRepo := repository.NewMCPRepository(pool)
	usageRepo := repository.NewUsageRepository(pool)
	budgetRepo := repository.NewBudgetRepository(pool)
	policyRepo := repository.NewPolicyRepository(pool)
	rbacRepo := repository.NewRBACRepository(pool)
	ssoRepo := repository.NewSSORepository(pool)
	auditRepo := repository.NewAuditRepository(pool)
	pricingRepo := repository.NewPricingRepository(pool)
	blogRepo := repository.NewBlogRepository(pool)
	pentestRunRepo := repository.NewPentestRunRepository(pool)
	pentestFindingRepo := repository.NewPentestFindingRepository(pool)
	reviewRunRepo := repository.NewReviewRunRepository(pool)
	taskRunRepo := repository.NewTaskRunRepository(pool)
	mailRepo := repository.NewMailRepository(pool)

	// Autonomous agents + chat + Telegram repositories
	memoryRepo := repository.NewMemoryRepository(pool)
	goalRepo := repository.NewGoalRepository(pool)
	multiAgentRepo := repository.NewMultiAgentRepository(pool)
	sprintRepo := repository.NewSprintRepository(pool)
	skillRepo := repository.NewSkillRepository(pool)
	chatRepo := repository.NewChatRepository(pool)
	telegramRepo := repository.NewTelegramRepository(pool)

	// ─── LLM layer ───────────────────────────────────────────────────────────
	// Ollama running locally is the default; OpenAI is used when configured.
	llmRouter := llm.NewRouter(cfg)
	logger.Info("LLM router initialised",
		zap.String("default_provider", cfg.LLMProvider),
		zap.String("ollama_url", cfg.OllamaBaseURL),
		zap.String("ollama_model", cfg.OllamaDefaultModel),
		// Whether the secret is set, never the secret itself.
		zap.Bool("ollama_gateway_auth", cfg.OllamaJWTSecret != ""),
		zap.Duration("ollama_timeout", cfg.OllamaTimeout),
		zap.Int("ollama_num_ctx", cfg.OllamaNumCtx),
	)

	// Langfuse tracing wraps every registered client before anything resolves
	// one, so a single call here covers the executor, blog, research and intent
	// paths. Chat uses a dedicated client (CHAT_MODEL), wrapped separately so
	// its DefaultModel is not the worker OLLAMA_DEFAULT_MODEL.
	chatInner := llm.NewChatInner(cfg, llmRouter)
	tracing := llmtrace.Init(cfg, logger)
	if tracing.Enabled() {
		llmRouter.WrapClients(tracing.Wrap)
		chatInner = tracing.Wrap(chatInner)
		logger.Info("LLM tracing enabled", zap.String("langfuse_host", cfg.LangfuseHost))
	}
	chatClient := llm.NewChatClient(chatInner, cfg.ChatModel, cfg.ChatModelFallback, logger)
	logger.Info("chat LLM client",
		zap.String("model", llm.SanitizeChatModel(cfg.ChatModel)),
		zap.String("fallback", llm.SanitizeChatFallback(cfg.ChatModelFallback)),
		zap.Int("num_ctx", cfg.ChatNumCtx),
	)

	// Warm the model-discovery cache so the picker and auto-selection have a
	// live answer from the first request, then keep it fresh in the background.
	// Best-effort throughout: a provider that cannot be probed degrades to its
	// static list rather than failing startup.
	startModelDiscovery(llmRouter, logger)

	// ─── Embedding + knowledge ingestion (RAG foundation) ────────────────────
	knowledgeChunkRepo := repository.NewKnowledgeChunkRepository(pool)
	var embedder llm.Embedder
	if e, err := llmRouter.Embedder(); err != nil {
		logger.Info("embeddings not configured — knowledge ingestion will be skipped",
			zap.String("embedding_provider", cfg.EmbeddingProvider), zap.Error(err))
	} else {
		embedder = e
		logger.Info("embedder initialised",
			zap.String("provider", e.EmbedderName()), zap.Int("dimensions", e.Dimensions()))
	}
	knowledgeIngestSvc := service.NewKnowledgeIngestService(knowledgeChunkRepo, embedder, logger)
	// Enable semantic long-term memory (embed-on-write + cosine recall); falls
	// back to ILIKE when embedder is nil.
	memoryRepo = memoryRepo.WithEmbedder(embedder)

	// ─── Object storage ─────────────────────────────────────────────────────
	// Built here rather than alongside the upload handler further down because
	// generated images need somewhere to live, and both the tool registry below
	// and the article generator after it need to know whether image generation
	// is available before they are assembled.
	var minioClient *miniogo.Client
	if cfg.MinIOEndpoint != "" {
		client, err := miniogo.New(cfg.MinIOEndpoint, &miniogo.Options{
			Creds:  credentials.NewStaticV4(cfg.MinIOAccessKey, cfg.MinIOSecretKey, ""),
			Secure: cfg.MinIOUseSSL,
		})
		if err != nil {
			logger.Warn("failed to create minio client — uploads and image storage disabled", zap.Error(err))
		} else {
			minioClient = client
		}
	}

	// ─── Image generation ───────────────────────────────────────────────────
	// The local provider runs on the workstation (see image-service/), outside
	// the cluster, because Apple MLX cannot be scheduled onto amd64 nodes —
	// the same arrangement Ollama already uses.
	imageRouter := imagegen.NewRouter(cfg).WithLogger(logger)
	var imgStore imagestore.Store
	if minioClient != nil {
		imgStore = imagestore.NewMinIOStore(minioClient, cfg.MinIOBucketImages)
	} else {
		// MinIO is optional locally. Without somewhere to write, the GPU still
		// draws covers and inline pictures, then the article pipeline drops them
		// because a cover with no URL is not a cover. A directory next to the
		// process is enough for the same /api/v1/images/file/… URLs.
		dir := os.Getenv("IMAGE_STORE_DIR")
		if dir == "" {
			dir = filepath.Join(".", ".dev-data", "images")
		}
		imgStore = imagestore.NewDirStore(dir)
		logger.Info("image storage using local directory (MinIO unset)", zap.String("dir", dir))
	}
	imageSvc := service.NewImageService(imageRouter, imgStore, repository.NewImageRepository(pool), logger)
	if imageSvc.Enabled() {
		logger.Info("image generation initialised",
			zap.Strings("providers", imageRouter.Providers()),
			zap.String("default_provider", imageRouter.DefaultProvider()),
			zap.Bool("storage", imgStore != nil),
			zap.Bool("blog_covers", cfg.BlogCoverImages))
		// Warm the model list so the first person to open the picker does not
		// wait on discovery. Bounded and best-effort: a workstation that is
		// asleep at boot must not hold up the server.
		warmCtx, cancelWarm := context.WithTimeout(ctx, 10*time.Second)
		go func() {
			defer cancelWarm()
			imageRouter.Warm(warmCtx)
		}()
	} else {
		logger.Info("image generation not configured — set GEMINI_API_KEY, IMAGE_BASE_URL or OPENAI_API_KEY to enable it")
	}

	// ─── Tool registry ───────────────────────────────────────────────────────
	toolRegistry := tools.NewRegistry()
	toolRegistry.Register(tools.NewHTTPTool())
	toolRegistry.Register(tools.NewShellTool(nil))
	// knowledge_search performs semantic retrieval over an agent's knowledge
	// base; it only works with a configured embedder, so register it only then.
	if embedder != nil {
		toolRegistry.Register(tools.NewKnowledgeTool(embedder, knowledgeChunkRepo))
	}
	// web_search / web_fetch / trending_topics give agents grounded internet
	// access. They need no credentials, so they register unconditionally — any
	// agent can be granted them through its tool permissions, and the Article
	// Writer is built on them.
	researchClient := research.New(logger, cfg.GitHubToken)
	for _, rt := range tools.NewResearchTools(researchClient) {
		toolRegistry.Register(rt)
	}
	// generate_image is registered whenever a provider is configured. An agent
	// granted a tool that always answers "not configured" learns nothing useful,
	// so the tool is absent rather than present-and-broken.
	if imageSvc.Enabled() {
		toolRegistry.Register(tools.NewGenerateImageTool(&toolImageGenerator{images: imageSvc}))
	}
	logger.Info("tool registry initialised", zap.Int("tools", len(toolRegistry.All())))

	// ─── Engine Router (multi-runtime) ──────────────────────────────────────
	// WithSkills folds each agent's enabled skills into its runs (prompt
	// patches + extra tools) via the skills registry.
	// WithKnowledge augments each run with the agent's most relevant stored
	// knowledge before the loop starts (best-effort; skipped when no embedder).
	goNativeExec := executor.New(llmRouter, toolRegistry, logger).
		WithSkills(skillRepo).
		WithKnowledge(knowledgeChunkRepo, embedder)

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

	// ─── Auto model selection ────────────────────────────────────────────────
	// Wired here rather than at executor construction because selection is
	// cost-aware and the cost engine does not exist until now. Agents pinned to
	// a provider ignore this entirely; only ModelProvider="auto" consults it.
	var autoSelector *modelselect.Selector
	if cfg.AutoModelSelection {
		// The dynamic catalog reads the router's discovery cache, so Auto can
		// reach the models actually installed rather than the single hardcoded
		// entry the static catalog carries. It uses the non-blocking accessor:
		// selection runs on the execution path and must never make a network
		// call. OllamaNumCtx is passed as the context ceiling so the selector
		// believes exactly what the client will request.
		autoSelector = modelselect.New(llmRouter, costEng, nil).
			WithDynamicCatalog(modelselect.LiveCatalog(llmRouter, cfg.OllamaNumCtx))
		goNativeExec.WithAutoSelect(autoSelector)
		logger.Info("auto model selection enabled")
	} else {
		logger.Info("auto model selection disabled; agents set to \"auto\" use the default provider")
	}

	// ─── Services ────────────────────────────────────────────────────────────
	jwtSvc := service.NewJWTService(cfg)
	authSvc := service.NewAuthService(userRepo, tokenRepo, orgRepo, agentRepo, rbacRepo, jwtSvc, logger)
	agentSvc := service.NewAgentService(agentRepo, logger)
	projectSvc := service.NewProjectService(projectRepo, logger)
	taskSvc := service.NewTaskService(taskRepo, logger)
	// Langfuse tracing for executions the Python sidecar does not see. Nil when
	// unconfigured, which disables tracing without any other code path caring.
	// Closed in the shutdown path below rather than deferred: the Fatal calls
	// on the way there use os.Exit, which would skip a defer anyway.
	tracer := langfuse.New(cfg.LangfuseHost, cfg.LangfusePublicKey, cfg.LangfuseSecretKey,
		cfg.LangfuseEnvironment, logger)

	govSvc := service.NewGovernanceService(budgetRepo, policyRepo, usageRepo, execRepo, costEng, tracer, logger)
	analyticsSvc := service.NewAnalyticsService(usageRepo, logger)
	rbacSvc := service.NewRBACService(rbacRepo, logger)
	ssoSvc := service.NewSSOService(ssoRepo, userRepo, rbacRepo, auditRepo, logger)
	leaderboardSvc := service.NewLeaderboardService(usageRepo, logger)
	execSvc := service.NewExecutionService(agentRepo, execRepo, toolPermRepo, engineRouter, govSvc, logger)
	taskRunSvc := service.NewTaskRunService(taskRunRepo, taskRepo, projectRepo, agentRepo, execSvc, logger)
	workflowSvc := service.NewWorkflowService(workflowRepo, agentRepo, execRepo, toolPermRepo, dagEngine, logger)
	pluginSvc := service.NewPluginService(pluginRepo, agentRepo, engineRouter, logger)

	// ─── Article generator (LLM → markdown → HTML → CMS draft) ──────────────
	// Built unconditionally: generation needs no opsapi credentials and its
	// output is read in the product. Only publishing is credential-gated, and
	// the runner reports that through CanPublish() so the UI can disable it.
	//
	// NewClient returns nil when the opsapi config is incomplete, which the
	// runner reads as "publishing is not configured".
	cmsClient := opsapi.NewClient(opsapi.Config{
		BaseURL:   cfg.OpsAPIBaseURL,
		APIKey:    cfg.OpsAPIKey,
		Namespace: cfg.OpsAPINamespace,
		Timeout:   cfg.OpsAPITimeout,
	})
	// The Research Agent shares the article generator's LLM but is wired
	// independently: it is a platform capability in its own right, and anything
	// that needs current, cited material about a subject consumes it.
	var researchAgent *research.Agent
	if researchLLM, err := llmRouter.For(cfg.LLMProvider); err != nil {
		logger.Warn("research: llm router returned error — research agent disabled", zap.Error(err))
	} else {
		researchAgent = research.NewAgent(researchClient, researchLLM, research.DefaultAgentConfig(), logger)
		logger.Info("research agent initialised",
			zap.Int("max_sources", research.DefaultAgentConfig().MaxSources))
	}
	researchSvc := service.NewResearchService(researchAgent, researchClient, agentRepo, logger)

	var blogRunner *blog.Runner
	if blogLLM, err := llmRouter.For(cfg.LLMProvider); err != nil {
		logger.Warn("blog: llm router returned error — article generator disabled",
			zap.Error(err))
	} else {
		blogRunner = blog.NewRunner(blog.Config{
			ContentDir:      cfg.BlogContentDir,
			AuthorName:      cfg.BlogAuthorName,
			PublicBaseURL:   cfg.FrontendBaseURL,
			Model:           cfg.BlogModel,
			ProseModel:      cfg.BlogProseModel,
			StructuredModel: cfg.BlogStructuredModel,
		}, blogLLM, cmsClient, researchSvc, logger)
		// Cover images and in-article illustrations are opt-in per environment:
		// each costs tens of seconds on a single shared GPU, so an operator
		// decides whether every article pays for one.
		if cfg.BlogCoverImages && imageSvc.Enabled() {
			blogRunner = blogRunner.WithIllustrator(&blogIllustrator{images: imageSvc})
		}
		writingModel := firstNonEmptyStr(cfg.BlogModel, cfg.OllamaDefaultModel)
		logger.Info("article generator initialised",
			zap.String("prose_model", firstNonEmptyStr(cfg.BlogProseModel, writingModel)),
			zap.String("structured_model", firstNonEmptyStr(cfg.BlogStructuredModel, writingModel)),
			zap.String("cms_namespace", cfg.OpsAPINamespace),
			zap.Bool("can_publish", blogRunner.CanPublish()),
		)
		if !blogRunner.CanPublish() {
			logger.Info("blog: opsapi CMS not configured — articles can be generated and read, but not published " +
				"(set OPSAPI_BASE_URL, OPSAPI_API_KEY and OPSAPI_NAMESPACE)")
		}
	}
	blogSvc := service.NewBlogService(
		blogRunner, blogRepo, researchSvc, agentRepo, logger,
		cfg.BlogOrphanTimeout, cfg.BlogMaxRuntime,
	)
	blogReconciler := service.NewBlogReconciler(blogSvc, 0, logger)

	// ─── Penetration Testing (Strix on the workstation) ──────────────────────
	// The scanner runs on the Mac Studio behind the same JWT-gated HTTP endpoint
	// as Ollama and the image service. CreateRun only queues a row; the
	// reconciler below claims queued runs off Postgres and drives them, so a
	// scan survives any deploy and multiple replicas share the work safely.
	strixConfig := strix.LoadConfig(logger)
	strixClient := strix.NewClient(strixConfig.BaseURL, strixConfig.JWTSecret, strixConfig.Timeout, logger)
	pentestSvc := service.NewPentestService(pentestRunRepo, pentestFindingRepo, agentRepo, strixClient, logger)
	pentestReconciler := service.NewPentestReconciler(pentestRunRepo, strixClient, strixConfig, logger)
	if strixConfig.Configured() {
		logger.Info("penetration testing enabled",
			zap.String("base_url", strixConfig.BaseURL),
			zap.Bool("gateway_auth", strixClient.UsesGateway()),
			zap.Int("allowlist_size", len(strixConfig.TargetAllowlist)),
			zap.Duration("poll_interval", strixConfig.PollInterval),
		)
	} else {
		logger.Info("penetration testing disabled (STRIX_BASE_URL empty)")
	}

	reviewCfg := reviewbot.LoadConfig(logger)
	reviewClient := reviewbot.NewClient(reviewCfg.BaseURL, reviewCfg.Token, reviewCfg.Timeout, logger)
	reviewSvc := service.NewReviewService(reviewRunRepo, reviewCfg, logger)
	reviewReconciler := service.NewReviewReconciler(reviewRunRepo, reviewClient, reviewCfg, logger)
	if reviewCfg.Configured() {
		toolRegistry.Register(tools.NewReviewPullRequestTool(reviewSvc))
		logger.Info("PR review enabled",
			zap.String("base_url", reviewCfg.BaseURL),
			zap.Int("allowlist_size", len(reviewCfg.AllowedRepos)),
			zap.Duration("poll_interval", reviewCfg.PollInterval),
		)
	} else {
		logger.Info("PR review disabled (REVIEW_BOT_BASE_URL empty)")
	}

	mailCfg := mail.LoadConfig()
	var mailLLM llm.Client
	if c, err := llmRouter.For(cfg.LLMProvider); err != nil {
		logger.Warn("mail: llm router returned error — classify/draft will use heuristics", zap.Error(err))
	} else {
		mailLLM = c
	}
	var gmailAPI mail.GmailAPI
	if mailCfg.Simulate {
		gmailAPI = mail.NewSimulatedGmail()
		logger.Warn("mail: MAIL_SIMULATE is on — inbox is fake, Google is not called")
	} else {
		gmailAPI = mail.NewGmailAPI(nil, logger)
	}
	mailSvc := service.NewMailService(
		mailRepo, agentRepo, gmailAPI,
		mail.NewClassifier(mailLLM, logger), mail.NewDrafter(mailLLM, mailCfg.DraftModel, logger),
		researchSvc, mailCfg, logger,
	)
	blogSvc.BindTasks(taskSvc)
	mailSvc.BindTasks(taskSvc)
	launchSvc := &tasklaunch.Service{
		Agents:   agentSvc,
		Tasks:    taskSvc,
		Projects: projectSvc,
		Research: researchSvc,
		Blog:     blogSvc,
		Mail:     mailSvc,
		Pentest:  pentestSvc,
		Reviews:  reviewSvc,
		Images:   imageSvc,
		TaskRuns: taskRunSvc,
	}
	mailReconciler := service.NewMailReconciler(mailSvc, mailCfg.ReconcileInterval, logger)
	if mailCfg.Configured() {
		logger.Info("mail agent oauth configured",
			zap.String("redirect_url", mailCfg.RedirectURL),
			zap.Duration("poll_interval", mailCfg.PollInterval),
		)
	} else {
		logger.Info("mail agent oauth not configured (set GMAIL_CLIENT_ID, GMAIL_CLIENT_SECRET, GMAIL_TOKEN_KEY)")
	}

	// ─── Autonomous agent engine ────────────────────────────────────────────
	autonomousExec := executor.NewAutonomousExecutor(goNativeExec, llmRouter, memoryRepo, goalRepo, logger).WithAutoSelect(autoSelector)
	memorySvc := service.NewMemoryService(memoryRepo, logger)
	goalSvc := service.NewGoalService(goalRepo, agentRepo, toolPermRepo, autonomousExec, logger)
	multiAgentSvc := service.NewMultiAgentService(multiAgentRepo, agentRepo, toolPermRepo, goalRepo, autonomousExec, logger)
	sprintSvc := service.NewSprintService(sprintRepo)

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

	// Phase 1: make the org's configured integrations agent-callable. These
	// tools resolve the calling org (from the execution context) to its own
	// Jira/GitHub/Slack/Teams/Email credentials at call time, so agents can act
	// on external systems inside the ReAct loop — not just background sync.
	for _, it := range tools.NewIntegrationTools(integRepo, notifConfigRepo, adapterRegistry) {
		toolRegistry.Register(it)
	}
	logger.Info("integration tools registered", zap.Int("count", 7))

	// Phase 1.2: make the org's configured MCP (Model Context Protocol) servers
	// agent-callable. mcp_list_tools and mcp_call resolve the calling org (from
	// the execution context) to its own enabled MCP servers at call time, so
	// agents can discover and invoke any MCP tool inside the ReAct loop.
	mcpSvc := service.NewMCPService(mcpRepo, logger)
	for _, it := range tools.NewMCPTools(mcpRepo) {
		toolRegistry.Register(it)
	}
	logger.Info("mcp tools registered", zap.Int("count", 2))

	budgetAlertDispatcher := service.NewBudgetAlertDispatcher(notifSvc, logger)
	_ = budgetAlertDispatcher // available for governance service to dispatch budget alerts
	go notifSvc.StartSubscriber(ctx, eventBus)

	// Phase 3: human-in-the-loop approvals. The approval service is both the
	// executor's approval gate (it decides which tool calls pause and records the
	// pending approval + manager notification) and the resume driver for
	// approve/reject decisions. Wire it as the go-native executor's gate now that
	// notifSvc exists; the gate is read at run time, so setting it post-construction
	// is safe and keeps runs default-off when no rule is configured.
	approvalSvc := service.NewApprovalService(approvalRepo, execRepo, agentRepo, userRepo, goNativeExec, notifSvc, logger)
	goNativeExec.WithApprovalGate(approvalSvc)
	logger.Info("human-in-the-loop approvals initialised")
	logger.Info("integration framework initialised",
		zap.Int("task_adapters", 2),
		zap.Int("notification_adapters", 3),
	)

	// ─── Chat agent (tool-calling loop over platform tools) ─────────────────
	var knowledgeSearch tools.KnowledgeSearcher
	if knowledgeChunkRepo != nil {
		knowledgeSearch = knowledgeChunkRepo
	}
	var toolEmbedder tools.Embedder
	if embedder != nil {
		toolEmbedder = embedder
	}
	platformReg := platformtools.NewRegistryWithTools(platformtools.Deps{
		Agents:          agentSvc,
		Exec:            execSvc,
		Workflows:       workflowSvc,
		Tasks:           taskSvc,
		Projects:        projectSvc,
		Goals:           goalSvc,
		Research:        researchSvc,
		Blog:            blogSvc,
		Pentest:         pentestSvc,
		Images:          imageSvc,
		Reviews:         reviewSvc,
		Mail:            mailSvc,
		MultiAgent:      multiAgentSvc,
		Sprints:         sprintSvc,
		Plugins:         pluginSvc,
		MCP:             mcpSvc,
		Integrations:    integSvc,
		Notifications:   notifSvc,
		Approvals:       approvalSvc,
		Analytics:       analyticsSvc,
		Leaderboard:     leaderboardSvc,
		Governance:      govSvc,
		RBAC:            rbacSvc,
		Scheduler:       schedulerRepo,
		Skills:          skillRepo,
		LLMProviders:    llmProviderRepo,
		Audit:           auditRepo,
		Sessions:        sessionRepo,
		Knowledge:       knowledgeIngestSvc,
		KnowledgeSearch: knowledgeSearch,
		Embedder:        toolEmbedder,
		Pool:            pool,
		Memory:          memorySvc,
		Launch:          launchSvc,
	})
	chatGuard := platformtools.NewGuard(rbacSvc, govSvc)
	chatAgent := chatagent.New(chatClient, platformReg, chatGuard, memorySvc, logger)
	chatSvc := chatsvc.NewChatService(chatRepo, chatAgent, logger)
	chatRouterSvc := chatsvc.NewChatRouterService(chatAgent, logger)
	logger.Info("chat agent initialised", zap.Int("platform_tools", len(platformReg.Names())))
	if chatClient != nil {
		if !chatClient.SupportsTools() {
			logger.Warn("chat agent: model has no native tool-calling — using ReAct fallback",
				zap.String("provider", chatClient.ProviderName()),
				zap.String("model", llm.SanitizeChatModel(cfg.ChatModel)))
		} else {
			logger.Info("chat agent: native tool-calling active",
				zap.String("provider", chatClient.ProviderName()),
				zap.String("model", llm.SanitizeChatModel(cfg.ChatModel)))
		}
	}

	// ─── Telegram bot (conditional on config) ───────────────────────────────
	// Telegram uses a deterministic session per chat ID, separate from web.
	var telegramSvc service.TelegramService
	var tgBot *telegramBot.BotClient
	if cfg.TelegramBotToken != "" {
		tgBot = telegramBot.NewBotClient(cfg.TelegramBotToken)
		telegramSvc = service.NewTelegramService(
			tgBot, telegramRepo, chatSvc,
			cfg.TelegramRatePerMin, cfg.FrontendBaseURL, logger,
		)
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

	// ─── Bridge client (SSE streaming) ──────────────────────────────────────
	var bridgeClient *bridge.Client
	if cfg.PythonSidecarURL != "" {
		bridgeClient = bridge.NewClient(cfg.PythonSidecarURL, cfg.PythonSidecarSecret, logger)
	}

	// ─── WebSocket hub ───────────────────────────────────────────────────────
	hub := ws.NewHub(logger)
	go hub.Run()

	// ─── Uploads ─────────────────────────────────────────────────────────────
	// The MinIO client itself is built earlier, where image storage needs it.
	var uploadHandler *handler.UploadHandler
	if minioClient != nil {
		uploadHandler = handler.NewUploadHandler(minioClient, cfg.MinIOBucketAvatars, logger)
	}

	// ─── Handlers ────────────────────────────────────────────────────────────
	authHandler := handler.NewAuthHandler(authSvc)
	imageHandler := handler.NewImageHandler(imageSvc)
	agentHandler := handler.NewAgentHandler(agentSvc)
	projectHandler := handler.NewProjectHandler(projectSvc)
	taskHandler := handler.NewTaskHandler(taskSvc, launchSvc)
	agentSchemaHandler := handler.NewAgentSchemaHandler()
	taskRunHandler := handler.NewTaskRunHandler(taskRunSvc)
	orgHandler := handler.NewOrganizationHandler(orgRepo)
	marketplaceHandler := handler.NewMarketplaceHandler(pool, logger)
	knowledgeHandler := handler.NewKnowledgeHandler(pool, knowledgeIngestSvc, logger)
	metricsHandler := handler.NewMetricsHandler(pool, logger)
	wsHandler := handler.NewWSHandler(hub, logger)
	execHandler := handler.NewExecutionHandler(execSvc)
	workflowHandler := handler.NewWorkflowHandler(workflowSvc)
	engineHandler := handler.NewEngineHandler(lcClient, lgClient, logger)
	pluginHandler := handler.NewPluginHandler(pluginSvc)
	streamHandler := handler.NewStreamHandler(bridgeClient, logger)
	llmProviderHandler := handler.NewLLMProviderHandler(llmProviderRepo, llmRouter, cfg.AutoModelSelection)
	schedulerHandler := handler.NewSchedulerHandler(schedulerRepo)
	sessionHandler := handler.NewSessionHandler(sessionRepo)
	integHandler := handler.NewIntegrationHandler(integSvc)
	mcpHandler := handler.NewMCPHandler(mcpSvc)
	notifHandler := handler.NewNotificationHandler(notifSvc)
	approvalHandler := handler.NewApprovalHandler(approvalSvc)
	webhookHandler := handler.NewWebhookHandler(integRepo, linkRepo, logger)
	governanceHandler := handler.NewGovernanceHandler(govSvc)
	analyticsHandler := handler.NewAnalyticsHandler(analyticsSvc)
	rbacHandler := handler.NewRBACHandler(rbacSvc)
	ssoHandler := handler.NewSSOHandler(ssoSvc, jwtSvc)
	auditHandler := handler.NewAuditHandler(auditRepo)
	pricingHandler := handler.NewPricingHandler(pricingRepo)
	leaderboardHandler := handler.NewLeaderboardHandler(leaderboardSvc)
	blogHandler := handler.NewBlogHandler(blogSvc)
	researchHandler := handler.NewResearchHandler(researchSvc)
	pentestHandler := handler.NewPentestHandler(pentestSvc)
	reviewHandler := handler.NewReviewHandler(reviewSvc)
	mailHandler := handler.NewMailHandler(mailSvc, mailCfg.FrontendBaseURL)

	// Chat, goal, multi-agent, and Telegram handlers
	chatHandler := handler.NewChatHandler(chatSvc)
	chatRouterHandler := handler.NewChatRouterHandler(chatRouterSvc)
	goalHandler := handler.NewGoalHandler(goalSvc)
	multiAgentHandler := handler.NewMultiAgentHandler(multiAgentSvc)
	sprintHandler := handler.NewSprintHandler(sprintSvc)
	skillHandler := handler.NewSkillHandler(skillRepo)
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
	r.Use(requestTimeout)

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

	// Health check — version/env/deployed_at feed the sidebar build stamp.
	r.Get("/health", handler.Health(pool, handler.RuntimeInfo{
		Version:    resolveVersion(),
		Env:        strings.TrimSpace(os.Getenv("APP_ENV")),
		DeployedAt: resolveDeployedAt(),
	}))

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
		// Google redirects the browser here with ?code=&state= — no JWT.
		r.Get("/mail/connection/oauth/callback", mailHandler.OAuthCallback)

		// Generated images are public. Keys embed UUIDs so they are not
		// enumerable, and Cache-Control already marks them immutable. Auth
		// would break opsapi's featured-image preview and any public site
		// that loads the cover with a plain <img> (no Authorization header).
		r.Get("/images/file/*", imageHandler.Serve)

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
			r.Get("/agent-schemas", agentSchemaHandler.List)
			r.Route("/tasks", func(r chi.Router) {
				r.Get("/", taskHandler.List)
				r.Post("/", taskHandler.Create)
				r.Post("/launch", taskHandler.Launch)
				r.Route("/{taskID}", func(r chi.Router) {
					r.Get("/", taskHandler.GetByID)
					r.Put("/", taskHandler.Update)
					r.Delete("/", taskHandler.Delete)
					r.Patch("/transition", taskHandler.Transition)
					r.Put("/position", taskHandler.Reorder)
					r.Get("/comments", taskHandler.ListComments)
					r.Post("/comments", taskHandler.AddComment)
					// On-demand agent runs of this task.
					r.Post("/run", taskRunHandler.CreateRun)
					r.Get("/runs", taskRunHandler.ListRuns)
				})
			})

			// A single task run, looked up by its own ID (the poll target).
			r.Get("/task-runs/{runID}", taskRunHandler.GetRun)

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

			// Article generator (LLM → markdown → optional git/PR). Generation
			// and publishing are separate so an article can be reviewed in the
			// UI before it reaches the content repository.
			r.Route("/blogs", func(r chi.Router) {
				r.Get("/config", blogHandler.Config)
				r.Post("/generate", blogHandler.Generate)
				r.Get("/runs", blogHandler.ListRuns)
				r.Route("/runs/{runID}", func(r chi.Router) {
					r.Get("/", blogHandler.GetRun)
					r.Get("/articles", blogHandler.ListArticles)
					r.Post("/publish", blogHandler.Publish)
					r.Post("/retry", blogHandler.Retry)
					r.Post("/cancel", blogHandler.Cancel)
					r.Delete("/", blogHandler.Delete)
				})
				r.Get("/articles/{articleID}", blogHandler.GetArticle)
			})

			// Research is exposed on its own, not only via the article
			// pipeline: "find out about X and come back with sources you have
			// actually read" is a capability other callers want too.
			r.Route("/research", func(r chi.Router) {
				// The long budget this route needs is applied by
				// requestTimeout below, not here — chi middleware nests rather
				// than overrides, so an inner Timeout cannot lengthen an outer
				// one that has already set a shorter deadline.
				r.Post("/", researchHandler.Research)
				r.Get("/trending", researchHandler.Trending)
			})

			// Penetration Testing (Strix)
			r.Route("/pentest", func(r chi.Router) {
				// Pre-flight: is the workstation ready to scan? Gates the Start button.
				r.Get("/capabilities", pentestHandler.GetCapabilities)
			})
			r.Route("/pentest-runs", func(r chi.Router) {
				r.Get("/", pentestHandler.ListRuns)
				r.Post("/", pentestHandler.CreateRun)
				r.Route("/{runID}", func(r chi.Router) {
					r.Get("/", pentestHandler.GetRun)
					r.Get("/findings", pentestHandler.ListFindings)
					r.Post("/cancel", pentestHandler.CancelRun)
				})
			})

			r.Route("/review-runs", func(r chi.Router) {
				r.Get("/repos", reviewHandler.ListRepos)
				r.Get("/", reviewHandler.ListRuns)
				r.Post("/", reviewHandler.CreateRun)
				r.Get("/{runID}", reviewHandler.GetRun)
			})

			r.Route("/mail", func(r chi.Router) {
				r.Get("/connection", mailHandler.GetConnection)
				r.Patch("/connection", mailHandler.PatchConnection)
				r.Delete("/connection", mailHandler.Disconnect)
				r.Post("/connection/oauth/start", mailHandler.StartOAuth)
				r.Post("/sync", mailHandler.Sync)
				r.Get("/threads", mailHandler.ListThreads)
				r.Get("/threads/{id}", mailHandler.GetThread)
				r.Get("/drafts", mailHandler.ListDrafts)
				r.Patch("/drafts/{id}", mailHandler.PatchDraft)
				r.Post("/drafts/{id}/approve", mailHandler.ApproveDraft)
				r.Post("/drafts/{id}/reject", mailHandler.RejectDraft)
				if mailCfg.Simulate {
					r.Post("/simulate/connect", mailHandler.SimulateConnect)
					r.Post("/simulate/inbox", mailHandler.SimulateInbox)
					r.Post("/simulate/sync", mailHandler.SimulateSync)
				}
			})

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
				// Models actually available to run, for the per-agent picker.
				r.Get("/models", llmProviderHandler.ListModels)
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

			// MCP servers (Model Context Protocol)
			r.Route("/mcp-servers", func(r chi.Router) {
				r.Get("/", mcpHandler.List)
				r.Post("/", mcpHandler.Create)
				r.Route("/{mcpID}", func(r chi.Router) {
					r.Get("/", mcpHandler.Get)
					r.Put("/", mcpHandler.Update)
					r.Delete("/", mcpHandler.Delete)
					r.Get("/tools", mcpHandler.ListTools)
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

			// Human-in-the-loop approvals (Phase 3)
			r.Route("/approvals", func(r chi.Router) {
				r.Get("/", approvalHandler.List)
				r.Route("/{approvalID}", func(r chi.Router) {
					r.Get("/", approvalHandler.Get)
					r.Post("/decide", approvalHandler.Decide)
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
					r.Delete("/", chatHandler.DeleteSession)
					r.Get("/messages", chatHandler.GetHistory)
					r.Post("/messages", chatHandler.SendMessage)
					r.Post("/messages/stream", chatHandler.StreamMessage)
				})
			})

			// Stateless chat router (Slack/Telegram/UI ad-hoc calls).
			r.Post("/chat/route", chatRouterHandler.Route)

			// Multi-agent collaboration jobs
			r.Route("/multi-agent/jobs", func(r chi.Router) {
				r.Get("/", multiAgentHandler.ListJobs)
				r.Post("/", multiAgentHandler.RunJob)
				r.Get("/{jobID}", multiAgentHandler.GetJob)
			})

			// Live agent board — current activity per agent (powers the
			// Kanban view in the dashboard).
			r.Get("/agents/board", multiAgentHandler.Board)

			// Skills registry (OpenClaw-style capability bundles)
			r.Route("/skills", func(r chi.Router) {
				r.Get("/", skillHandler.List)
				r.Post("/", skillHandler.Create)
				r.Get("/{skillID}", skillHandler.Get)
				r.Put("/{skillID}", skillHandler.Update)
				r.Delete("/{skillID}", skillHandler.Delete)
			})
			// Per-agent skill enablement
			r.Get("/agents/{agentID}/skills", skillHandler.ListForAgent)
			r.Post("/agents/{agentID}/skills", skillHandler.EnableForAgent)
			r.Delete("/agents/{agentID}/skills/{skillID}", skillHandler.DisableForAgent)

			// Sprints (Scrum-style iteration board)
			r.Route("/sprints", func(r chi.Router) {
				r.Get("/", sprintHandler.List)
				r.Post("/", sprintHandler.Create)
				r.Route("/{sprintID}", func(r chi.Router) {
					r.Get("/", sprintHandler.Get)
					r.Put("/", sprintHandler.Update)
					r.Delete("/", sprintHandler.Delete)
					r.Post("/jobs", sprintHandler.AddJob)
					r.Delete("/jobs/{jobID}", sprintHandler.RemoveJob)
					r.Post("/agents", sprintHandler.AddAgent)
					r.Delete("/agents/{agentID}", sprintHandler.RemoveAgent)
				})
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

			// Image generation. Registered unconditionally: /models answers
			// "enabled: false" when nothing is configured, which the UI renders
			// as a disabled control — a 404 there would look like a broken
			// deployment rather than a switched-off feature. File serving is
			// registered above, outside auth, so CMS covers stay loadable.
			r.Route("/images", func(r chi.Router) {
				r.Get("/models", imageHandler.ListModels)
				r.Post("/generate", imageHandler.Generate)
				r.Get("/", imageHandler.List)
			})

			// WebSocket
			r.Get("/ws", wsHandler.Connect)
		})
	})

	// ─── Scheduler dispatcher ───────────────────────────────────────────────
	// Ticks every 30s, picks up due scheduled_tasks, and dispatches them to
	// the appropriate path (blog pipeline / workflow / agent).
	schedulerRunner := scheduler.NewRunner(schedulerRepo, blogSvc, workflowSvc, execSvc, multiAgentSvc, logger)
	go schedulerRunner.Start(ctx)

	// ─── Pentest reconciler ─────────────────────────────────────────────────
	// Ticks every STRIX_POLL_INTERVAL, claims due pentest runs with FOR UPDATE
	// SKIP LOCKED, and advances each by polling the workstation service. A no-op
	// when STRIX_BASE_URL is unset. Stops when ctx is cancelled on shutdown.
	go pentestReconciler.Start(ctx)
	// Same durable-queue shape as pentest: Postgres is the system of record,
	// the ClusterIP sidecar holds in-memory OpenCode jobs.
	go reviewReconciler.Start(ctx)
	go mailReconciler.Start(ctx)

	// ─── Blog orphan reconciler ─────────────────────────────────────────────
	// Fails running rows whose writer died (SIGKILL, OOM, node drain). SIGTERM
	// is handled by InterruptAll below; this loop covers the rest. Does not
	// restart generation — Retry is the user's action.
	go blogReconciler.Start(ctx)

	srv := &http.Server{
		Addr:        cfg.ServerPort,
		Handler:     r,
		ReadTimeout: 15 * time.Second,
		// Must exceed the longest per-route handler deadline (research or sync
		// image generation), or the response is cut off after the work is done.
		WriteTimeout: max(researchRequestTimeout, imageRequestTimeout, chatRequestTimeout) + time.Minute,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in a goroutine
	go func() {
		logger.Info("starting server",
			zap.String("port", cfg.ServerPort),
			zap.String("version", resolveVersion()),
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
	// Fail in-flight article runs before the process dies, otherwise they stay
	// `running` forever and the UI cannot Retry or Delete them. stopping is
	// set first so a Generate that is still inside its HTTP handler cannot
	// start a new goroutine after we have cancelled the ones we know about.
	blogSvc.InterruptAll(nil)
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Fatal("server forced shutdown", zap.Error(err))
	}

	// Flush queued spans before exit. Without this the last few traces of a
	// deploy are lost, which is exactly when they are most worth having.
	tracer.Close()
	if err := tracing.Shutdown(shutdownCtx); err != nil {
		logger.Warn("langfuse flush failed", zap.Error(err))
	}

	logger.Info("server stopped")
}

// modelDiscoveryInterval is how often the model list is re-probed. Models are
// installed rarely, so this only needs to be faster than a user's patience after
// running `ollama pull`.
const modelDiscoveryInterval = 5 * time.Minute

// startModelDiscovery warms the router's model cache and refreshes it on a
// ticker.
//
// The warm-up is synchronous but bounded, so the first request to the picker
// does not pay for discovery; the refresh runs for the process lifetime. Every
// failure is logged and swallowed — a provider that cannot be reached degrades
// to its static model list, which is far better than refusing to start.
func startModelDiscovery(router *llm.Router, logger *zap.Logger) {
	warmCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for _, pm := range router.RefreshModels(warmCtx) {
		fields := []zap.Field{
			zap.String("provider", pm.Provider),
			zap.String("source", pm.Source),
			zap.Int("models", len(pm.Models)),
		}
		if pm.Error != "" {
			logger.Warn("model discovery degraded", append(fields, zap.String("error", pm.Error))...)
			continue
		}
		logger.Info("model discovery", fields...)
	}

	go func() {
		ticker := time.NewTicker(modelDiscoveryInterval)
		defer ticker.Stop()
		for range ticker.C {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			router.RefreshModels(ctx)
			cancel()
		}
	}()
}

func resolveVersion() string {
	if v := strings.TrimSpace(os.Getenv("APP_VERSION")); v != "" && v != "latest" {
		return v
	}
	if v := strings.TrimSpace(version); v != "" && v != "dev" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv("APP_VERSION")); v != "" {
		return v
	}
	if v := strings.TrimSpace(version); v != "" {
		return v
	}
	return "dev"
}

func resolveDeployedAt() time.Time {
	if s := strings.TrimSpace(os.Getenv("APP_DEPLOYED_AT")); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			return t
		}
	}
	return startedAt
}
