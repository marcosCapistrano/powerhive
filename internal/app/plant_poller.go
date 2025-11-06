package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"powerhive/internal/config"
	"powerhive/internal/database"
)

const plantRequestTimeout = 10 * time.Second

// PlantPoller periodically fetches energy generation and consumption data from the hydro plant API.
type PlantPoller struct {
	store      *database.Store
	cfg        config.AppConfig
	log        *slog.Logger
	httpClient *http.Client
	interval   time.Duration
}

// NewPlantPoller creates a new plant data polling service.
func NewPlantPoller(store *database.Store, cfg config.AppConfig, logger *slog.Logger) *PlantPoller {
	return &PlantPoller{
		store:      store,
		cfg:        cfg,
		log:        logger.With("component", "plant"),
		httpClient: &http.Client{Timeout: plantRequestTimeout},
		interval:   time.Duration(cfg.Intervals.PlantSeconds) * time.Second,
	}
}

// Run starts the plant data polling loop.
func (p *PlantPoller) Run(ctx context.Context) {
	p.log.Info("starting plant polling loop", "interval", p.interval)

	// Initial poll
	if err := p.poll(ctx); err != nil {
		p.log.Error("initial plant poll failed", "err", err)
	}

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			p.log.Info("stopping plant polling loop", "reason", ctx.Err())
			return
		case <-ticker.C:
			if err := p.poll(ctx); err != nil {
				p.log.Error("plant poll failed", "err", err)
			}
		}
	}
}

func (p *PlantPoller) poll(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.cfg.Plant.APIEndpoint, nil)
	if err != nil {
		return fmt.Errorf("create plant request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", p.cfg.Plant.APIKey))
	req.Header.Set("Accept", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetch plant data: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("plant API returned status %d", resp.StatusCode)
	}

	var apiResp PlantAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return fmt.Errorf("decode plant response: %w", err)
	}

	if !apiResp.Success {
		return fmt.Errorf("plant API returned success=false")
	}

	if len(apiResp.Data) == 0 {
		p.log.Warn("plant API returned no data")
		return nil
	}

	// Get the most recent reading (first in array)
	latest := apiResp.Data[0]

	// Calculate total container consumption
	totalContainerConsumption := 0.0
	for _, consumption := range latest.ContainerConsumption {
		totalContainerConsumption += consumption
	}

	// Calculate available power (generation - container consumption)
	availablePower := latest.TotalGeneration - totalContainerConsumption

	// Store raw JSON for debugging
	rawJSON, _ := json.Marshal(apiResp)
	rawStr := string(rawJSON)

	input := database.PlantReadingInput{
		PlantID:                   latest.PlantID,
		TotalGeneration:           latest.TotalGeneration,
		TotalContainerConsumption: totalContainerConsumption,
		AvailablePower:            availablePower,
		RawData:                   &rawStr,
		RecordedAt:                latest.Timestamp,
	}

	reading, err := p.store.RecordPlantReading(ctx, input)
	if err != nil {
		return fmt.Errorf("store plant reading: %w", err)
	}

	p.log.Info("plant data recorded",
		"plant_id", reading.PlantID,
		"generation_kw", reading.TotalGeneration,
		"container_kw", reading.TotalContainerConsumption,
		"available_kw", reading.AvailablePower,
	)

	return nil
}

// PlantAPIResponse models the response from the energy aggregator API.
type PlantAPIResponse struct {
	Success bool               `json:"success"`
	Count   int                `json:"count"`
	Data    []PlantDataReading `json:"data"`
}

// PlantDataReading represents a single plant energy reading from the API.
type PlantDataReading struct {
	Timestamp            time.Time              `json:"timestamp"`
	PlantID              string                 `json:"plant_id"`
	TotalGeneration      float64                `json:"total_generation"`
	ContainerConsumption map[string]float64     `json:"container_consumption"`
}
