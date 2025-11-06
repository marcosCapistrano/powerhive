package app

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"powerhive/internal/config"
	"powerhive/internal/database"
	"powerhive/internal/server"
)

const (
	httpWriteTimeout = 30 * time.Second
	httpReadTimeout  = 10 * time.Second
	httpIdleTimeout  = 60 * time.Second
	shutdownTimeout  = 5 * time.Second
)

// App orchestrates background services and the dashboard server.
type App struct {
	cfg          config.AppConfig
	log          *slog.Logger
	discovery    *Discoverer
	status       *StatusPoller
	telemetry    *TelemetryPoller
	plantPoller  *PlantPoller
	powerBalancer *PowerBalancer
	server       *server.Server
	httpServer   *http.Server
}

// New builds an App with all dependencies wired.
func New(cfg config.AppConfig, store *database.Store, logger *slog.Logger) (*App, error) {
	if logger == nil {
		logger = slog.Default()
	}

	discovery := NewDiscoverer(store, cfg, logger)
	status := NewStatusPoller(store, cfg, logger)
	telemetry := NewTelemetryPoller(store, cfg, logger)
	plantPoller := NewPlantPoller(store, cfg, logger)
	powerBalancer := NewPowerBalancer(store, cfg, logger)

	srv, err := server.New(store, logger)
	if err != nil {
		return nil, err
	}

	httpServer := &http.Server{
		Addr:              cfg.HTTP.Addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: httpReadTimeout,
		WriteTimeout:      httpWriteTimeout,
		IdleTimeout:       httpIdleTimeout,
	}

	return &App{
		cfg:          cfg,
		log:          logger.With("component", "app"),
		discovery:    discovery,
		status:       status,
		telemetry:    telemetry,
		plantPoller:  plantPoller,
		powerBalancer: powerBalancer,
		server:       srv,
		httpServer:   httpServer,
	}, nil
}

// Run starts the services and blocks until the context is cancelled or an error occurs.
func (a *App) Run(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, 1)
	var wg sync.WaitGroup

	startService := func(name string, run func(context.Context)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a.log.Info("service started", "service", name)
			run(ctx)
			a.log.Info("service stopped", "service", name)
		}()
	}

	startService("discovery", a.discovery.Run)
	startService("status", a.status.Run)
	startService("telemetry", a.telemetry.Run)
	startService("plant_poller", a.plantPoller.Run)
	startService("power_balancer", a.powerBalancer.Run)

	wg.Add(1)
	go func() {
		defer wg.Done()
		a.log.Info("http listening", "addr", a.cfg.HTTP.Addr)
		if err := a.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	var runErr error
	select {
	case <-ctx.Done():
		runErr = ctx.Err()
	case err := <-errCh:
		runErr = err
		cancel()
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer shutdownCancel()
	if err := a.httpServer.Shutdown(shutdownCtx); err != nil && !errors.Is(err, context.Canceled) {
		a.log.Error("http shutdown failed", "err", err)
	}

	cancel()
	wg.Wait()

	if runErr != nil && !errors.Is(runErr, context.Canceled) {
		return runErr
	}
	return nil
}
