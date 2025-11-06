package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"powerhive/internal/config"
	"powerhive/internal/database"
	"powerhive/internal/firmware"
)

const (
	telemetryWorkerCount = 4
)

// TelemetryPoller captures chip-level telemetry on a slower cadence.
type TelemetryPoller struct {
	store        *database.Store
	cfg          config.AppConfig
	log          *slog.Logger
	httpClient   *http.Client
	interval     time.Duration
	requestLimit time.Duration
}

// NewTelemetryPoller constructs a telemetry polling service.
func NewTelemetryPoller(store *database.Store, cfg config.AppConfig, logger *slog.Logger) *TelemetryPoller {
	if logger == nil {
		logger = slog.Default()
	}

	timeout := time.Duration(cfg.Network.MinerProbeTimeoutMs) * time.Millisecond

	return &TelemetryPoller{
		store:        store,
		cfg:          cfg,
		log:          logger.With("component", "telemetry"),
		httpClient:   &http.Client{Timeout: timeout},
		interval:     time.Duration(cfg.Intervals.TelemetrySeconds) * time.Second,
		requestLimit: timeout,
	}
}

// Run starts the telemetry loop until cancellation.
func (p *TelemetryPoller) Run(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}

	p.log.Info("starting telemetry loop", "interval", p.interval)

	if err := p.poll(ctx); err != nil {
		p.log.Error("initial telemetry poll failed", "err", err)
	}

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			p.log.Info("stopping telemetry loop", "reason", ctx.Err())
			return
		case <-ticker.C:
			if err := p.poll(ctx); err != nil {
				p.log.Error("telemetry poll failed", "err", err)
			}
		}
	}
}

func (p *TelemetryPoller) poll(ctx context.Context) error {
	miners, err := p.store.ListMiners(ctx)
	if err != nil {
		return fmt.Errorf("list miners: %w", err)
	}

	type target struct {
		miner database.Miner
	}

	var targets []target
	for _, miner := range miners {
		if !miner.Managed {
			continue
		}
		if miner.Model == nil || miner.Model.MaxPreset == nil {
			continue
		}
		if miner.IP == nil || strings.TrimSpace(*miner.IP) == "" {
			continue
		}
		if miner.APIKey == nil || strings.TrimSpace(*miner.APIKey) == "" {
			continue
		}
		targets = append(targets, target{miner: miner})
	}

	if len(targets) == 0 {
		return nil
	}

	type telemetryResult struct {
		miner database.Miner
		data  []firmware.ChainTelemetry
		err   error
	}

	resultCh := make(chan telemetryResult, len(targets))
	jobs := make(chan target)

	var wg sync.WaitGroup
	workers := telemetryWorkerCount
	if len(targets) < workers {
		workers = len(targets)
	}
	if workers < 1 {
		workers = 1
	}

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				select {
				case <-ctx.Done():
					return
				default:
				}

				miner := job.miner
				client, err := firmware.NewClient(*miner.IP,
					firmware.WithHTTPClient(p.httpClient),
					firmware.WithAPIKey(strings.TrimSpace(*miner.APIKey)))
				if err != nil {
					resultCh <- telemetryResult{miner: miner, err: fmt.Errorf("create client: %w", err)}
					continue
				}

				reqCtx, cancel := context.WithTimeout(ctx, p.requestLimit)
				chains, err := client.Chains(reqCtx)
				cancel()
				if err != nil {
					resultCh <- telemetryResult{miner: miner, err: fmt.Errorf("fetch chains: %w", err)}
					continue
				}

				resultCh <- telemetryResult{miner: miner, data: chains}
			}
		}()
	}

	go func() {
		for _, target := range targets {
			select {
			case <-ctx.Done():
				close(jobs)
				return
			case jobs <- target:
			}
		}
		close(jobs)
	}()

	go func() {
		wg.Wait()
		close(resultCh)
	}()

	for res := range resultCh {
		if res.err != nil {
			p.log.Warn("telemetry poll failed", "miner", res.miner.ID, "ip", safeString(res.miner.IP), "err", res.err)
			continue
		}
		if err := p.persistTelemetry(ctx, res.miner, res.data); err != nil {
			p.log.Warn("persist telemetry failed", "miner", res.miner.ID, "err", err)
		}
	}

	return nil
}

func (p *TelemetryPoller) persistTelemetry(ctx context.Context, miner database.Miner, chains []firmware.ChainTelemetry) error {
	if len(chains) == 0 {
		return nil
	}

	var snapshots []database.ChainSnapshotInput
	for _, chain := range chains {
		identifier := fmt.Sprintf("chain-%d", chain.ID)
		snapshot := database.ChainSnapshotInput{
			ChainIdentifier: stringPtr(identifier),
			State:           stringPtr(strings.TrimSpace(chain.Status.State)),
			Hashrate:        chain.HashrateRealtime,
		}

		for _, chip := range chain.Chips {
			chipID := fmt.Sprintf("chip-%d", chip.ID)
			snapshot.Chips = append(snapshot.Chips, database.ChipSnapshotInput{
				ChipIdentifier: stringPtr(chipID),
				Hashrate:       chip.Hashrate,
				Temperature:    chip.Temp,
			})
		}

		snapshots = append(snapshots, snapshot)
	}

	if err := p.store.RecordChainTelemetry(ctx, miner.ID, time.Now().UTC(), snapshots); err != nil {
		return fmt.Errorf("record chain telemetry: %w", err)
	}

	p.log.Debug("telemetry recorded", "miner", miner.ID, "chains", len(snapshots))
	return nil
}
