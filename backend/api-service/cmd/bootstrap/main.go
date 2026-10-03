package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"sourcecraft.dev/lct-hackaton-2026/case-19-negotiation-simulator-team-83/backend/api-service/internal/config"
	"sourcecraft.dev/lct-hackaton-2026/case-19-negotiation-simulator-team-83/backend/api-service/internal/database"
	"sourcecraft.dev/lct-hackaton-2026/case-19-negotiation-simulator-team-83/backend/api-service/internal/repository"
	"sourcecraft.dev/lct-hackaton-2026/case-19-negotiation-simulator-team-83/backend/api-service/internal/services/auth"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	const op = "bootstrap.run"

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("%s: load configuration: %w", op, err)
	}

	logger := setupLogger(cfg.Env)
	slog.SetDefault(logger)

	log := logger.With(slog.String("op", op))

	db, err := database.Connect(cfg.DB)
	if err != nil {
		log.Error("connect to database", "error", err)

		return fmt.Errorf("%s: connect to database: %w", op, err)
	}

	if err := database.Migrate(context.Background(), db); err != nil {
		log.Error("migrate database", "error", err)

		return fmt.Errorf("%s: migrate database: %w", op, err)
	}

	service := auth.NewAuthService(
		repository.NewUserRepo(*db, logger),
		repository.NewTokenRepo(*db, logger),
		cfg,
		db,
		logger,
	)

	companyID, err := service.BootstrapAdmin(context.Background(), os.Getenv("BOOTSTRAP_COMPANY_NAME"), os.Getenv("BOOTSTRAP_ADMIN_EMAIL"), os.Getenv("BOOTSTRAP_ADMIN_NAME"), os.Getenv("BOOTSTRAP_ADMIN_PASSWORD"))
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	fmt.Println(companyID)

	return nil
}

func setupLogger(env string) *slog.Logger {
	var log *slog.Logger

	switch env {
	case config.EnvLocal:
		log = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	case config.EnvDev:
		log = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	case config.EnvProd:
		log = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	}

	return log
}
