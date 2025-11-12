package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"powerhive/internal/app"
	"powerhive/internal/config"
	"powerhive/internal/database"

	_ "modernc.org/sqlite"
)

func main() {
	// Parse command-line flags
	testMode := flag.Bool("test", false, "Enable test mode (POST expected consumption to test server)")
	testServerURL := flag.String("test-server-url", "", "Test server URL (overrides config, e.g., http://localhost:8090)")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	cfg, err := config.Load("config.json")
	if err != nil {
		logger.Error("load config failed", "err", err)
		os.Exit(1)
	}

	// Override config with command-line flags
	if *testMode {
		cfg.Plant.TestMode = true
		logger.Info("test mode enabled")
	}
	if *testServerURL != "" {
		cfg.Plant.TestServerURL = *testServerURL
		logger.Info("test server URL overridden", "url", *testServerURL)
	}

	db, err := sql.Open("sqlite", cfg.Database.Path)
	if err != nil {
		logger.Error("open database failed", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	// Configure connection pool for SQLite single-writer constraint
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	if err := db.Ping(); err != nil {
		logger.Error("ping database failed", "err", err)
		os.Exit(1)
	}

	store, err := database.New(db)
	if err != nil {
		logger.Error("configure database failed", "err", err)
		os.Exit(1)
	}

	if err := store.Init(context.Background()); err != nil {
		logger.Error("initialise schema failed", "err", err)
		os.Exit(1)
	}

	appInstance, err := app.New(cfg, store, logger)
	if err != nil {
		logger.Error("initialise app failed", "err", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger.Info("powerhive starting", "database", cfg.Database.Path, "http_addr", cfg.HTTP.Addr)

	if err := appInstance.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("app terminated with error", "err", err)
		os.Exit(1)
	}

	logger.Info("powerhive stopped")
}
