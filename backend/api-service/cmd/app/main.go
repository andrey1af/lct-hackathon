package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"sourcecraft.dev/lct-hackaton-2026/case-19-negotiation-simulator-team-83/backend/api-service/internal/llm"
	"sourcecraft.dev/lct-hackaton-2026/case-19-negotiation-simulator-team-83/backend/api-service/internal/middleware"
	"sourcecraft.dev/lct-hackaton-2026/case-19-negotiation-simulator-team-83/backend/api-service/internal/negotiation"
	"sourcecraft.dev/lct-hackaton-2026/case-19-negotiation-simulator-team-83/backend/api-service/internal/repository"
	"sourcecraft.dev/lct-hackaton-2026/case-19-negotiation-simulator-team-83/backend/api-service/internal/services/auth"
	handlers "sourcecraft.dev/lct-hackaton-2026/case-19-negotiation-simulator-team-83/backend/api-service/internal/transport/http"

	"sourcecraft.dev/lct-hackaton-2026/case-19-negotiation-simulator-team-83/backend/api-service/internal/config"
	"sourcecraft.dev/lct-hackaton-2026/case-19-negotiation-simulator-team-83/backend/api-service/internal/content"
	"sourcecraft.dev/lct-hackaton-2026/case-19-negotiation-simulator-team-83/backend/api-service/internal/database"
	"sourcecraft.dev/lct-hackaton-2026/case-19-negotiation-simulator-team-83/backend/api-service/internal/services/learning"
)

const (
	envLocal = config.EnvLocal
	envDev   = config.EnvDev
	envProd  = config.EnvProd
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Default().Error("load configuration", "error", err)
		os.Exit(1)
	}

	logger := setupLogger(cfg.Env)
	slog.SetDefault(logger)

	if err := run(cfg, logger); err != nil {
		logger.Error("application stopped", "error", err)
		os.Exit(1)
	}
}

func setupLogger(env string) *slog.Logger {
	var log *slog.Logger

	switch env {
	case envLocal:
		log = slog.New(
			middleware.RequestIDHandler(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})),
		)

	case envDev:
		log = slog.New(
			middleware.RequestIDHandler(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})),
		)

	case envProd:
		log = slog.New(
			middleware.RequestIDHandler(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})),
		)
	}

	return log
}

func run(cfg config.Config, logger *slog.Logger) error {
	const op = "app.run"

	log := logger.With(slog.String("op", op))

	db, err := database.Connect(cfg.DB)
	if err != nil {
		log.Error("connect to database", "error", err)

		return fmt.Errorf("%s: connect to database: %w", op, err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		log.Error("get underlying sql.DB", "error", err)

		return fmt.Errorf("%s: get underlying sql.DB: %w", op, err)
	}

	defer func() {
		if closeErr := sqlDB.Close(); closeErr != nil {
			log.Error("failed to close database connection", "error", closeErr)
		}
	}()

	log.Info("database connection established", "env", cfg.Env, "host", cfg.DB.Host, "port", cfg.DB.Port, "name", cfg.DB.Name)

	if err := database.Migrate(context.Background(), db); err != nil {
		log.Error("migrate database", "error", err)

		return fmt.Errorf("%s: migrate database: %w", op, err)
	}

	contentDir := os.Getenv("CONTENT_DIR")
	if contentDir == "" {
		contentDir = "content"
	}

	seedData, err := content.Load(contentDir)
	if err != nil {
		log.Error("load content", "error", err, "dir", contentDir)

		return fmt.Errorf("%s: load content: %w", op, err)
	}

	if err := content.Seed(context.Background(), db, seedData); err != nil {
		log.Error("seed content", "error", err)

		return fmt.Errorf("%s: seed content: %w", op, err)
	}

	openAPISpec, err := os.ReadFile(cfg.HTTP.OpenAPISpecPath)
	if err != nil {
		log.Error("read openapi spec", "error", err, "path", cfg.HTTP.OpenAPISpecPath)

		return fmt.Errorf("%s: read openapi spec: %w", op, err)
	}

	healthHandler := handlers.NewHealthHandler(sqlDB, cfg.HTTP.HealthTimeout)
	learningHandler := handlers.NewLearningHandler(learning.NewService(db, logger))

	llmProvider := llm.New(cfg.GigaChat, logger)
	negotiationHandler := handlers.NewNegotiationHandler(
		negotiation.NewService(db, llmProvider, negotiation.Config{DefaultCaseKey: cfg.Negotiation.DefaultCaseKey}, logger),
	)

	userRepo := repository.NewUserRepo(*db, logger)
	tokenRepo := repository.NewTokenRepo(*db, logger)
	userService := auth.NewAuthService(userRepo, tokenRepo, cfg, db, logger)
	authMiddleware := middleware.RequireAccessToken(userService)
	companyMiddleware := middleware.RequireCompanyPrincipal(userService)
	userHandler := handlers.NewUserHandler(userService)
	platformHandler := handlers.NewPlatformHandler(userService)
	server := &http.Server{
		Addr: net.JoinHostPort("", strconv.Itoa(cfg.HTTP.Port)),
		Handler: middleware.RequestLogger(logger)(
			handlers.NewRouter(healthHandler, cfg.HTTP.CORSAllowedOrigins, openAPISpec, learningHandler, negotiationHandler, authMiddleware, companyMiddleware, userHandler, platformHandler),
		),
		ReadHeaderTimeout: cfg.HTTP.ReadHeaderTimeout,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)

	defer stop()

	serverErr := make(chan error, 1)

	go func() {
		log.Info("HTTP server started", "port", cfg.HTTP.Port)

		serverErr <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}

		log.Error("HTTP server stopped", "error", err)

		return fmt.Errorf("%s: %w", op, err)

	case <-ctx.Done():
		log.Info("shutdown signal received")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.HTTP.ShutdownTimeout)

		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Error("failed to shutdown", "error", err)

			return fmt.Errorf("%s: shutdown: %w", op, err)
		}

		log.Info("HTTP server stopped gracefully")
	}

	return nil
}
