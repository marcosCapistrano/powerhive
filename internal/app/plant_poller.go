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

	reading := apiResp.Reading

	// Check confidence score - skip readings with confidence ≤ 0.8
	if reading.Trust.ConfidenceScore <= 0.8 {
		p.log.Warn("skipping low confidence reading",
			"confidence", reading.Trust.ConfidenceScore,
			"status", reading.Trust.Status,
			"plant_id", reading.PlantID,
		)
		return nil
	}

	// Extract individual source data (already in MW from API)
	generationSources := make(map[string]float64)
	for sourceName, source := range reading.Generation {
		if source.Status == "success" {
			generationSources[sourceName] = source.ValueMW
		}
	}

	consumptionSources := make(map[string]float64)
	for sourceName, source := range reading.Consumption {
		if source.Status == "success" {
			consumptionSources[sourceName] = source.ValueMW
		}
	}

	// Convert totals from MW to kW (multiply by 1000)
	totalGenerationKW := reading.Totals.GenerationMW * 1000
	totalConsumptionKW := reading.Totals.ConsumptionMW * 1000

	// Calculate available power (generation - consumption) in kW
	availablePowerKW := totalGenerationKW - totalConsumptionKW

	// Store raw JSON for debugging
	rawJSON, _ := json.Marshal(apiResp)
	rawStr := string(rawJSON)

	input := database.PlantReadingInput{
		PlantID:                   reading.PlantID,
		TotalGeneration:           totalGenerationKW,
		TotalContainerConsumption: totalConsumptionKW,
		AvailablePower:            availablePowerKW,
		GenerationSources:         generationSources,
		ConsumptionSources:        consumptionSources,
		RawData:                   &rawStr,
		RecordedAt:                reading.CollectionTimestamp,
	}

	stored, err := p.store.RecordPlantReading(ctx, input)
	if err != nil {
		return fmt.Errorf("store plant reading: %w", err)
	}

	p.log.Info("plant data recorded",
		"plant_id", stored.PlantID,
		"generation_kw", stored.TotalGeneration,
		"container_kw", stored.TotalContainerConsumption,
		"available_kw", stored.AvailablePower,
		"confidence", reading.Trust.ConfidenceScore,
	)

	return nil
}

// PlantAPIResponse models the response from the energy aggregator API.
type PlantAPIResponse struct {
	Reading PlantDataReading `json:"reading"`
}

// PlantDataReading represents a single plant energy reading from the API.
type PlantDataReading struct {
	ID                  int                       `json:"id"`
	PlantID             string                    `json:"plant_id"`
	CollectionTimestamp time.Time                 `json:"collection_timestamp"`
	Generation          map[string]SourceReading  `json:"generation"`
	Consumption         map[string]SourceReading  `json:"consumption"`
	Totals              PlantTotals               `json:"totals"`
	Trust               TrustInfo                 `json:"trust"`
}

// SourceReading represents an individual energy source (generator or container) with metadata.
type SourceReading struct {
	SourceTimestamp time.Time `json:"source_timestamp"`
	Status          string    `json:"status"`
	ValueMW         float64   `json:"value_mw"`
}

// PlantTotals contains pre-calculated aggregate values.
type PlantTotals struct {
	GenerationMW  float64 `json:"generation_mw"`
	ConsumptionMW float64 `json:"consumption_mw"`
	ExportedMW    float64 `json:"exported_mw"`
}

// TrustInfo contains confidence scoring for the reading.
type TrustInfo struct {
	ConfidenceScore float64 `json:"confidence_score"`
	Status          string  `json:"status"`
	Summary         string  `json:"summary"`
}
