package app

import (
	"context"
	"encoding/json"
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
	statusWorkerCount = 4
)

// StatusPoller captures periodic miner summaries.
type StatusPoller struct {
	store        *database.Store
	cfg          config.AppConfig
	log          *slog.Logger
	httpClient   *http.Client
	interval     time.Duration
	requestLimit time.Duration
}

// NewStatusPoller creates a status polling service.
func NewStatusPoller(store *database.Store, cfg config.AppConfig, logger *slog.Logger) *StatusPoller {
	if logger == nil {
		logger = slog.Default()
	}

	timeout := time.Duration(cfg.Network.MinerProbeTimeoutMs) * time.Millisecond

	return &StatusPoller{
		store:        store,
		cfg:          cfg,
		log:          logger.With("component", "status"),
		httpClient:   &http.Client{Timeout: timeout},
		interval:     time.Duration(cfg.Intervals.StatusSeconds) * time.Second,
		requestLimit: timeout,
	}
}

// Run starts the polling loop until the context is cancelled.
func (p *StatusPoller) Run(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}

	p.log.Info("starting status loop", "interval", p.interval)

	if err := p.poll(ctx); err != nil {
		p.log.Error("initial status poll failed", "err", err)
	}

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			p.log.Info("stopping status loop", "reason", ctx.Err())
			return
		case <-ticker.C:
			if err := p.poll(ctx); err != nil {
				p.log.Error("status poll failed", "err", err)
			}
		}
	}
}

func (p *StatusPoller) poll(ctx context.Context) error {
	miners, err := p.store.ListMiners(ctx)
	if err != nil {
		return fmt.Errorf("list miners: %w", err)
	}

	type pollTarget struct {
		miner database.Miner
	}

	var targets []pollTarget
	for _, miner := range miners {
		if !miner.Managed {
			continue
		}
		if miner.IP == nil || strings.TrimSpace(*miner.IP) == "" {
			continue
		}
		if miner.APIKey == nil || strings.TrimSpace(*miner.APIKey) == "" {
			continue
		}
		targets = append(targets, pollTarget{miner: miner})
	}

	if len(targets) == 0 {
		return nil
	}

	type pollResult struct {
		miner   database.Miner
		summary firmware.SummaryResponse
		preset  *string
		err     error
	}

	resultCh := make(chan pollResult, len(targets))
	jobs := make(chan pollTarget)

	var wg sync.WaitGroup
	workers := statusWorkerCount
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
					resultCh <- pollResult{miner: miner, err: fmt.Errorf("create client: %w", err)}
					continue
				}

				reqCtx, cancel := context.WithTimeout(ctx, p.requestLimit)
				summary, err := client.Summary(reqCtx)
				cancel()
				if err != nil {
					resultCh <- pollResult{miner: miner, err: fmt.Errorf("fetch summary: %w", err)}
					continue
				}

				var preset *string
				perfCtx, cancelPerf := context.WithTimeout(ctx, p.requestLimit)
				perfSummary, perfErr := client.PerfSummary(perfCtx)
				cancelPerf()
				if perfErr != nil {
					p.log.Debug("perf summary fetch failed", "miner", miner.ID, "err", perfErr)
				} else {
					preset = parseCurrentPreset(perfSummary.CurrentPreset)
				}

				resultCh <- pollResult{miner: miner, summary: summary, preset: preset}
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
			p.log.Warn("poll miner failed", "miner", res.miner.ID, "ip", safeString(res.miner.IP), "err", res.err)
			continue
		}
		if err := p.persistStatus(ctx, res.miner, res.summary, res.preset); err != nil {
			p.log.Warn("persist miner status failed", "miner", res.miner.ID, "err", err)
		}
	}

	return nil
}

func (p *StatusPoller) persistStatus(ctx context.Context, miner database.Miner, summary firmware.SummaryResponse, preset *string) error {
	state := strings.TrimSpace(summary.Miner.MinerStatus.MinerState)
	var statePtr *string
	if state != "" {
		statePtr = &state
	}

	statusInput := database.MinerStatusInput{
		Uptime:           summary.Miner.MinerStatus.MinerStateTime,
		State:            statePtr,
		Preset:           preset,
		Hashrate:         summary.Miner.HashrateRealtime,
		PowerUsage:       summary.Miner.PowerUsage,
		PowerConsumption: summary.Miner.PowerConsumption,
		RecordedAt:       time.Now().UTC(),
	}

	for _, fan := range summary.Miner.Cooling.Fans {
		fanID := fmt.Sprintf("fan-%d", fan.ID)
		statusInput.Fans = append(statusInput.Fans, database.FanStatusInput{
			FanIdentifier: stringPtr(fanID),
			RPM:           fan.RPM,
			Status:        stringPtr(strings.TrimSpace(fan.Status)),
		})
	}

	for _, chain := range summary.Miner.Chains {
		identifier := fmt.Sprintf("chain-%d", chain.ID)
		snapshot := database.ChainSnapshotInput{
			ChainIdentifier: stringPtr(identifier),
			State:           stringPtr(strings.TrimSpace(chain.Status.State)),
			Hashrate:        chain.HashrateRealtime,
			PCBTempMin:      chain.PCBTemp.Min,
			PCBTempMax:      chain.PCBTemp.Max,
			ChipTempMin:     chain.ChipTemp.Min,
			ChipTempMax:     chain.ChipTemp.Max,
		}
		statusInput.Chains = append(statusInput.Chains, snapshot)
	}

	if _, err := p.store.RecordMinerStatus(ctx, miner.ID, statusInput); err != nil {
		return fmt.Errorf("record status: %w", err)
	}

	p.log.Debug("miner status recorded", "miner", miner.ID, "hashrate", valueOrZero(summary.Miner.HashrateRealtime))
	return nil
}

func parseCurrentPreset(raw json.RawMessage) *string {
	if len(raw) == 0 {
		return nil
	}

	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return stringPtr(asString)
	}

	var preset struct {
		Name   string `json:"name"`
		Pretty string `json:"pretty"`
		Preset string `json:"preset"`
	}
	if err := json.Unmarshal(raw, &preset); err == nil {
		if ptr := stringPtr(preset.Name); ptr != nil {
			return ptr
		}
		if ptr := stringPtr(preset.Preset); ptr != nil {
			return ptr
		}
		if ptr := stringPtr(preset.Pretty); ptr != nil {
			return ptr
		}
	}

	str := strings.TrimSpace(string(raw))
	if str == "" || strings.EqualFold(str, "null") {
		return nil
	}
	if strings.HasPrefix(str, "\"") && strings.HasSuffix(str, "\"") {
		str = strings.Trim(str, "\"")
	}
	return stringPtr(str)
}

func stringPtr(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func valueOrZero(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}

func safeString(ptr *string) string {
	if ptr == nil {
		return ""
	}
	return *ptr
}
