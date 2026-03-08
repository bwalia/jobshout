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

	"github.com/jobshout/server/internal/config"
	"github.com/jobshout/server/internal/database"
	"github.com/jobshout/server/internal/handler"
	"github.com/jobshout/server/internal/middleware"
	"github.com/jobshout/server/internal/repository"
	"github.com/jobshout/server/internal/service"
	ws "github.com/jobshout/server/internal/websocket"
)

const version = "0.1.0"

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

	// Repositories
	userRepo := repository.NewUserRepository(pool)
	tokenRepo := repository.NewTokenRepository(pool)
	orgRepo := repository.NewOrganizationRepository(pool)
	agentRepo := repository.NewAgentRepository(pool)
	projectRepo := repository.NewProjectRepository(pool)
	taskRepo := repository.NewTaskRepository(pool)

	// Services
	jwtSvc := service.NewJWTService(cfg)
	authSvc := service.NewAuthService(userRepo, tokenRepo, orgRepo, jwtSvc, logger)
	agentSvc := service.NewAgentService(agentRepo, logger)
	projectSvc := service.NewProjectService(projectRepo, logger)
	taskSvc := service.NewTaskService(taskRepo, logger)

	// WebSocket hub
	hub := ws.NewHub(logger)
	go hub.Run()

	// MinIO client (optional — if endpoint is configured)
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

	// Handlers
	authHandler := handler.NewAuthHandler(authSvc)
	agentHandler := handler.NewAgentHandler(agentSvc)
	projectHandler := handler.NewProjectHandler(projectSvc)
	taskHandler := handler.NewTaskHandler(taskSvc)
	orgHandler := handler.NewOrganizationHandler(orgRepo)
	marketplaceHandler := handler.NewMarketplaceHandler(pool, logger)
	knowledgeHandler := handler.NewKnowledgeHandler(pool, logger)
	metricsHandler := handler.NewMetricsHandler(pool, logger)
	wsHandler := handler.NewWSHandler(hub, logger)

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
				})
			})

			// Projects
			r.Route("/projects", func(r chi.Router) {
				r.Get("/", projectHandler.List)
				r.Post("/", projectHandler.Create)
				r.Route("/{projectID}", func(r chi.Router) {
					r.Get("/", projectHandler.GetByID)
					r.Put("/", projectHandler.Update)
					r.Delete("/", projectHandler.Delete)
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
		WriteTimeout: 15 * time.Second,
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
