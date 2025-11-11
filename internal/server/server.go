package server

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"powerhive/internal/database"
)

// Server exposes the dashboard API and static assets.
type Server struct {
	store  *database.Store
	log    *slog.Logger
	mux    *http.ServeMux
	static http.Handler
}

// New constructs a Server with routes configured.
func New(store *database.Store, logger *slog.Logger) (*Server, error) {
	if logger == nil {
		logger = slog.Default()
	}

	static, err := staticHandler()
	if err != nil {
		return nil, fmt.Errorf("prepare static assets: %w", err)
	}

	s := &Server{
		store:  store,
		log:    logger.With("component", "http"),
		mux:    http.NewServeMux(),
		static: static,
	}

	s.routes()
	return s, nil
}

// Handler exposes the configured mux for use with http.Server.
func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) routes() {
	s.mux.Handle("/api/miners", http.HandlerFunc(s.handleMiners))
	s.mux.Handle("/api/miners/", http.HandlerFunc(s.handleMinerRoutes))

	s.mux.Handle("/api/models", http.HandlerFunc(s.handleModels))
	s.mux.Handle("/api/models/", http.HandlerFunc(s.handleModelRoutes))

	s.mux.Handle("/api/plant/latest", http.HandlerFunc(s.handlePlantLatest))
	s.mux.Handle("/api/plant/history", http.HandlerFunc(s.handlePlantHistory))

	s.mux.Handle("/api/balance/events", http.HandlerFunc(s.handleBalanceEvents))
	s.mux.Handle("/api/balance/status", http.HandlerFunc(s.handleBalanceStatus))

	s.mux.Handle("/api/settings", http.HandlerFunc(s.handleSettings))
	s.mux.Handle("/api/settings/", http.HandlerFunc(s.handleSettingsRoutes))

	// Static assets and dashboard.
	s.mux.Handle("/", http.HandlerFunc(s.handleStatic))
}

func (s *Server) handleMiners(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listMiners(w, r)
	default:
		methodNotAllowed(w, http.MethodGet)
	}
}

func (s *Server) handleMinerRoutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/miners/")
	if path == "" {
		http.NotFound(w, r)
		return
	}

	parts := strings.Split(path, "/")
	minerID := strings.ToLower(strings.TrimSpace(parts[0]))
	if minerID == "" {
		http.NotFound(w, r)
		return
	}

	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			s.getMiner(w, r, minerID)
		case http.MethodPatch:
			s.updateMiner(w, r, minerID)
		default:
			methodNotAllowed(w, http.MethodGet, http.MethodPatch)
		}
		return
	}

	switch parts[1] {
	case "statuses":
		if r.Method == http.MethodGet {
			s.listMinerStatuses(w, r, minerID)
			return
		}
		methodNotAllowed(w, http.MethodGet)
	case "telemetry":
		if r.Method == http.MethodGet {
			s.listMinerTelemetry(w, r, minerID)
			return
		}
		methodNotAllowed(w, http.MethodGet)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listModels(w, r)
	default:
		methodNotAllowed(w, http.MethodGet)
	}
}

func (s *Server) handleModelRoutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/models/")
	alias := strings.TrimSpace(path)
	if alias == "" {
		http.NotFound(w, r)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.getModel(w, r, alias)
	case http.MethodPatch:
		s.updateModel(w, r, alias)
	default:
		methodNotAllowed(w, http.MethodGet, http.MethodPatch)
	}
}

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		http.NotFound(w, r)
		return
	}
	s.static.ServeHTTP(w, r)
}

func (s *Server) listMiners(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	miners, err := s.store.ListMiners(ctx)
	if err != nil {
		s.log.Error("list miners failed", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to list miners")
		return
	}

	out := make([]minerDTO, 0, len(miners))
	for _, miner := range miners {
		out = append(out, toMinerDTO(miner))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) getMiner(w http.ResponseWriter, r *http.Request, minerID string) {
	ctx := r.Context()
	miner, err := s.store.GetMiner(ctx, minerID)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "miner not found")
			return
		}
		s.log.Error("get miner failed", "miner", minerID, "err", err)
		writeError(w, http.StatusInternalServerError, "failed to fetch miner")
		return
	}

	writeJSON(w, http.StatusOK, toMinerDTO(miner))
}

func (s *Server) updateMiner(w http.ResponseWriter, r *http.Request, minerID string) {
	ctx := r.Context()
	var req updateMinerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}
	if req.Managed == nil && req.UnlockPass == nil {
		writeError(w, http.StatusBadRequest, "no fields to update")
		return
	}

	params := database.UpsertMinerParams{
		ID: strings.ToLower(minerID),
	}

	if req.Managed != nil {
		params.Managed = req.Managed
	}

	if req.UnlockPass != nil {
		pass := strings.TrimSpace(*req.UnlockPass)
		if pass == "" {
			writeError(w, http.StatusBadRequest, "unlock_pass cannot be empty")
			return
		}
		params.UnlockPass = &pass
	}

	if _, err := s.store.UpsertMiner(ctx, params); err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "miner not found")
			return
		}
		s.log.Error("update miner failed", "miner", minerID, "err", err)
		writeError(w, http.StatusInternalServerError, "failed to update miner")
		return
	}

	updated, err := s.store.GetMiner(ctx, minerID)
	if err != nil {
		s.log.Error("rehydrate miner failed", "miner", minerID, "err", err)
		writeError(w, http.StatusInternalServerError, "failed to fetch updated miner")
		return
	}

	writeJSON(w, http.StatusOK, toMinerDTO(updated))
}

func (s *Server) listMinerStatuses(w http.ResponseWriter, r *http.Request, minerID string) {
	ctx := r.Context()
	limit := 10
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	statuses, err := s.store.ListMinerStatuses(ctx, minerID, limit)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "miner not found")
			return
		}
		s.log.Error("list statuses failed", "miner", minerID, "err", err)
		writeError(w, http.StatusInternalServerError, "failed to fetch statuses")
		return
	}

	out := make([]statusDTO, 0, len(statuses))
	for _, status := range statuses {
		out = append(out, toStatusDTO(status))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) listMinerTelemetry(w http.ResponseWriter, r *http.Request, minerID string) {
	ctx := r.Context()
	limit := 30
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	snapshots, err := s.store.ListChainTelemetry(ctx, minerID, limit)
	if err != nil {
		s.log.Error("list telemetry failed", "miner", minerID, "err", err)
		writeError(w, http.StatusInternalServerError, "failed to fetch telemetry")
		return
	}

	out := make([]chainTelemetryDTO, 0, len(snapshots))
	for _, snapshot := range snapshots {
		out = append(out, toChainTelemetryDTO(snapshot))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) listModels(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	models, err := s.store.ListModels(ctx)
	if err != nil {
		s.log.Error("list models failed", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to list models")
		return
	}

	// Load all preset power data in a single query
	powerDataMap, err := s.store.GetAllModelPresets(ctx)
	if err != nil {
		s.log.Warn("failed to load preset power data", "err", err)
		powerDataMap = make(map[string][]database.ModelPreset)
	}

	out := make([]modelDTO, 0, len(models))
	for _, model := range models {
		dto := toModelDTO(model)

		// Use preset power data from bulk query
		if presetsPower, ok := powerDataMap[model.Alias]; ok {
			dto.PresetsPower = make([]presetPowerDTO, 0, len(presetsPower))
			for _, pp := range presetsPower {
				dto.PresetsPower = append(dto.PresetsPower, presetPowerDTO{
					Preset: pp.Value,
					PowerW: pp.ExpectedPowerW,
				})
			}
		}

		out = append(out, dto)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) getModel(w http.ResponseWriter, r *http.Request, alias string) {
	ctx := r.Context()
	model, err := s.store.GetModelByAlias(ctx, alias)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "model not found")
			return
		}
		s.log.Error("get model failed", "alias", alias, "err", err)
		writeError(w, http.StatusInternalServerError, "failed to fetch model")
		return
	}

	dto := toModelDTO(model)

	// Load preset power data
	presetsPower, err := s.store.GetModelPresets(ctx, model.Alias)
	if err != nil {
		s.log.Warn("failed to load preset power", "model", model.Alias, "err", err)
	} else {
		dto.PresetsPower = make([]presetPowerDTO, 0, len(presetsPower))
		for _, pp := range presetsPower {
			dto.PresetsPower = append(dto.PresetsPower, presetPowerDTO{
				Preset: pp.Value,
				PowerW: pp.ExpectedPowerW,
			})
		}
	}

	writeJSON(w, http.StatusOK, dto)
}

func (s *Server) updateModel(w http.ResponseWriter, r *http.Request, alias string) {
	ctx := r.Context()
	model, err := s.store.GetModelByAlias(ctx, alias)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "model not found")
			return
		}
		s.log.Error("get model for update failed", "alias", alias, "err", err)
		writeError(w, http.StatusInternalServerError, "failed to fetch model")
		return
	}

	var req updateModelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}

	if !req.HasUpdates() {
		writeError(w, http.StatusBadRequest, "no fields to update")
		return
	}

	var maxPreset *string
	if req.MaxPreset != nil {
		value := strings.TrimSpace(*req.MaxPreset)
		if value == "" {
			maxPreset = nil
		} else {
			if !containsCaseInsensitive(model.Presets, value) {
				writeError(w, http.StatusBadRequest, "max_preset must match an available preset")
				return
			}
			maxPreset = &value
		}
	} else {
		maxPreset = model.MaxPreset
	}

	updated, err := s.store.UpsertModel(ctx, database.ModelInput{
		Name:      model.Name,
		Alias:     model.Alias,
		MaxPreset: maxPreset,
	})
	if err != nil {
		s.log.Error("update model failed", "alias", alias, "err", err)
		writeError(w, http.StatusInternalServerError, "failed to update model")
		return
	}

	writeJSON(w, http.StatusOK, toModelDTO(updated))
}

func methodNotAllowed(w http.ResponseWriter, allowed ...string) {
	w.Header().Set("Allow", strings.Join(allowed, ", "))
	writeError(w, http.StatusMethodNotAllowed, "method not allowed")
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if payload == nil {
		return
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	type errorResponse struct {
		Error string `json:"error"`
	}
	writeJSON(w, status, errorResponse{Error: message})
}

func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "not found")
}

func containsCaseInsensitive(values []string, target string) bool {
	target = strings.TrimSpace(strings.ToLower(target))
	for _, value := range values {
		if strings.ToLower(strings.TrimSpace(value)) == target {
			return true
		}
	}
	return false
}

type updateMinerRequest struct {
	Managed    *bool   `json:"managed"`
	UnlockPass *string `json:"unlock_pass"`
}

type updateModelRequest struct {
	MaxPreset *string `json:"max_preset"`
}

func (r *updateModelRequest) HasUpdates() bool {
	return r.MaxPreset != nil
}

type minerDTO struct {
	ID           string     `json:"id"`
	IP           *string    `json:"ip"`
	Online       bool       `json:"online"`
	Managed      bool       `json:"managed"`
	Model        *modelDTO  `json:"model,omitempty"`
	LatestStatus *statusDTO `json:"latest_status,omitempty"`
	CreatedAt    string     `json:"created_at"`
	UpdatedAt    string     `json:"updated_at"`
}

type modelDTO struct {
	Name         string        `json:"name"`
	Alias        string        `json:"alias"`
	MaxPreset    *string       `json:"max_preset"`
	Presets      []string      `json:"presets"`
	PresetsPower []presetPowerDTO `json:"presets_power"`
	CreatedAt    string        `json:"created_at"`
}

type presetPowerDTO struct {
	Preset string   `json:"preset"`
	PowerW *float64 `json:"power_w"`
}

type statusDTO struct {
	ID               int64      `json:"id"`
	State            *string    `json:"state"`
	Preset           *string    `json:"preset"`
	Hashrate         *float64   `json:"hashrate"`
	PowerUsage       *float64   `json:"power_usage"`
	PowerConsumption *float64   `json:"power_consumption"`
	Uptime           *int64     `json:"uptime"`
	RecordedAt       string     `json:"recorded_at"`
	Fans             []fanDTO   `json:"fans,omitempty"`
	Chains           []chainDTO `json:"chains,omitempty"`
}

type fanDTO struct {
	Identifier *string `json:"identifier"`
	RPM        *int    `json:"rpm"`
	Status     *string `json:"status"`
}

type chainDTO struct {
	Identifier  *string   `json:"identifier"`
	State       *string   `json:"state"`
	Hashrate    *float64  `json:"hashrate"`
	PCBTempMin  *float64  `json:"pcb_temp_min"`
	PCBTempMax  *float64  `json:"pcb_temp_max"`
	ChipTempMin *float64  `json:"chip_temp_min"`
	ChipTempMax *float64  `json:"chip_temp_max"`
	Chips       []chipDTO `json:"chips,omitempty"`
}

type chipDTO struct {
	Identifier  *string  `json:"identifier"`
	Hashrate    *float64 `json:"hashrate"`
	Temperature *float64 `json:"temperature"`
}

type chainTelemetryDTO struct {
	ID         int64    `json:"id"`
	StatusID   *int64   `json:"status_id,omitempty"`
	RecordedAt string   `json:"recorded_at"`
	Chain      chainDTO `json:"chain"`
}

func toMinerDTO(miner database.Miner) minerDTO {
	var model *modelDTO
	if miner.Model != nil {
		model = &modelDTO{
			Name:      miner.Model.Name,
			Alias:     miner.Model.Alias,
			MaxPreset: miner.Model.MaxPreset,
			Presets:   append([]string{}, miner.Model.Presets...),
			CreatedAt: formatTime(miner.Model.CreatedAt),
		}
	}

	var latest *statusDTO
	if miner.LatestStatus != nil {
		dto := toStatusDTO(*miner.LatestStatus)
		latest = &dto
	}

	return minerDTO{
		ID:           miner.ID,
		IP:           miner.IP,
		Online:       miner.IP != nil && strings.TrimSpace(*miner.IP) != "",
		Managed:      miner.Managed,
		Model:        model,
		LatestStatus: latest,
		CreatedAt:    formatTime(miner.CreatedAt),
		UpdatedAt:    formatTime(miner.UpdatedAt),
	}
}

func toModelDTO(model database.Model) modelDTO {
	return modelDTO{
		Name:      model.Name,
		Alias:     model.Alias,
		MaxPreset: model.MaxPreset,
		Presets:   append([]string{}, model.Presets...),
		CreatedAt: formatTime(model.CreatedAt),
	}
}

func toStatusDTO(status database.Status) statusDTO {
	dto := statusDTO{
		ID:               status.ID,
		State:            status.State,
		Preset:           status.Preset,
		Hashrate:         status.Hashrate,
		PowerUsage:       status.PowerUsage,
		PowerConsumption: status.PowerConsumption,
		Uptime:           status.Uptime,
		RecordedAt:       formatTime(status.RecordedAt),
	}

	for _, fan := range status.Fans {
		dto.Fans = append(dto.Fans, fanDTO{
			Identifier: fan.FanIdentifier,
			RPM:        fan.RPM,
			Status:     fan.Status,
		})
	}

	for _, chain := range status.Chains {
		c := chainDTO{
			Identifier:  chain.ChainIdentifier,
			State:       chain.State,
			Hashrate:    chain.Hashrate,
			PCBTempMin:  chain.PCBTempMin,
			PCBTempMax:  chain.PCBTempMax,
			ChipTempMin: chain.ChipTempMin,
			ChipTempMax: chain.ChipTempMax,
		}
		for _, chip := range chain.Chips {
			c.Chips = append(c.Chips, chipDTO{
				Identifier:  chip.ChipIdentifier,
				Hashrate:    chip.Hashrate,
				Temperature: chip.Temperature,
			})
		}
		dto.Chains = append(dto.Chains, c)
	}

	return dto
}

func toChainTelemetryDTO(snapshot database.ChainSnapshot) chainTelemetryDTO {
	entry := chainTelemetryDTO{
		ID:         snapshot.ID,
		RecordedAt: formatTime(snapshot.RecordedAt),
		Chain: chainDTO{
			Identifier: snapshot.ChainIdentifier,
			State:      snapshot.State,
			Hashrate:   snapshot.Hashrate,
		},
	}

	if snapshot.StatusID != nil {
		entry.StatusID = snapshot.StatusID
	}

	for _, chip := range snapshot.Chips {
		entry.Chain.Chips = append(entry.Chain.Chips, chipDTO{
			Identifier:  chip.ChipIdentifier,
			Hashrate:    chip.Hashrate,
			Temperature: chip.Temperature,
		})
	}

	return entry
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// Plant energy handlers

func (s *Server) handlePlantLatest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}

	ctx := r.Context()
	reading, err := s.store.GetLatestPlantReading(ctx)
	if err != nil {
		s.log.Error("get latest plant reading failed", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to fetch plant data")
		return
	}

	if reading == nil {
		writeJSON(w, http.StatusOK, nil)
		return
	}

	writeJSON(w, http.StatusOK, toPlantReadingDTO(*reading))
}

func (s *Server) handlePlantHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}

	ctx := r.Context()
	limit := 100
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	readings, err := s.store.ListPlantReadings(ctx, limit)
	if err != nil {
		s.log.Error("list plant readings failed", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to fetch plant history")
		return
	}

	out := make([]plantReadingDTO, 0, len(readings))
	for _, reading := range readings {
		out = append(out, toPlantReadingDTO(reading))
	}
	writeJSON(w, http.StatusOK, out)
}

// Balance event handlers

func (s *Server) handleBalanceEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}

	ctx := r.Context()
	limit := 100
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	minerID := r.URL.Query().Get("miner_id")
	var minerIDPtr *string
	if minerID != "" {
		minerIDPtr = &minerID
	}

	events, err := s.store.ListPowerBalanceEvents(ctx, minerIDPtr, limit)
	if err != nil {
		s.log.Error("list balance events failed", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to fetch balance events")
		return
	}

	out := make([]powerBalanceEventDTO, 0, len(events))
	for _, event := range events {
		out = append(out, toPowerBalanceEventDTO(event))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleBalanceStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}

	ctx := r.Context()

	// Get latest plant reading
	plantReading, err := s.store.GetLatestPlantReading(ctx)
	if err != nil {
		s.log.Error("get plant reading failed", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to fetch balance status")
		return
	}

	// Get safety margin
	safetyMarginStr, err := s.store.GetAppSetting(ctx, "safety_margin_percent")
	if err != nil {
		s.log.Warn("get safety margin failed, using default", "err", err)
		safetyMarginStr = "10.0"
	}

	var safetyMargin float64
	if err := json.Unmarshal([]byte(safetyMarginStr), &safetyMargin); err != nil {
		safetyMargin = 10.0
	}

	// Get all managed miners and calculate current consumption
	miners, err := s.store.ListMiners(ctx)
	if err != nil {
		s.log.Error("list miners failed", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to fetch balance status")
		return
	}

	var currentConsumption float64
	managedCount := 0
	for _, miner := range miners {
		if !miner.Managed {
			continue
		}
		managedCount++
		if miner.LatestStatus != nil && miner.LatestStatus.PowerConsumption != nil {
			currentConsumption += *miner.LatestStatus.PowerConsumption
		}
	}

	var status balanceStatusDTO
	status.SafetyMarginPercent = safetyMargin
	status.ManagedMinersCount = managedCount
	status.CurrentConsumptionW = currentConsumption

	if plantReading != nil {
		status.PlantGenerationKW = plantReading.TotalGeneration
		status.PlantContainerKW = plantReading.TotalContainerConsumption
		status.AvailablePowerKW = plantReading.AvailablePower
		targetPower := plantReading.TotalGeneration * (1.0 - safetyMargin/100.0)
		status.TargetPowerKW = targetPower
		status.TargetPowerW = targetPower * 1000.0

		// Include last reading timestamp
		timestamp := formatTime(plantReading.RecordedAt)
		status.LastReadingAt = &timestamp

		// Calculate status
		delta := (status.TargetPowerW - currentConsumption) / status.TargetPowerW * 100
		if delta < -5 {
			status.Status = "OVER_TARGET"
		} else if delta < 0 {
			status.Status = "WARNING"
		} else {
			status.Status = "OK"
		}
	} else {
		status.Status = "NO_DATA"
	}

	writeJSON(w, http.StatusOK, status)
}

// Settings handlers

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listSettings(w, r)
	default:
		methodNotAllowed(w, http.MethodGet)
	}
}

func (s *Server) handleSettingsRoutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/settings/")
	if path == "" {
		http.NotFound(w, r)
		return
	}

	if path == "safety-margin" {
		switch r.Method {
		case http.MethodPatch:
			s.updateSafetyMargin(w, r)
		default:
			methodNotAllowed(w, http.MethodPatch)
		}
		return
	}

	http.NotFound(w, r)
}

func (s *Server) listSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	safetyMarginStr, err := s.store.GetAppSetting(ctx, "safety_margin_percent")
	if err != nil {
		s.log.Warn("get safety margin failed, using default", "err", err)
		safetyMarginStr = "10.0"
	}

	var safetyMargin float64
	if err := json.Unmarshal([]byte(safetyMarginStr), &safetyMargin); err != nil {
		safetyMargin = 10.0
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"safety_margin_percent": safetyMargin,
	})
}

func (s *Server) updateSafetyMargin(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req struct {
		SafetyMarginPercent float64 `json:"safety_margin_percent"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.SafetyMarginPercent < 0 || req.SafetyMarginPercent > 50 {
		writeError(w, http.StatusBadRequest, "safety margin must be between 0 and 50 percent")
		return
	}

	valueJSON, err := json.Marshal(req.SafetyMarginPercent)
	if err != nil {
		s.log.Error("marshal safety margin failed", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to update setting")
		return
	}

	if err := s.store.SetAppSetting(ctx, "safety_margin_percent", string(valueJSON)); err != nil {
		s.log.Error("set safety margin failed", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to update setting")
		return
	}

	s.log.Info("safety margin updated", "new_value", req.SafetyMarginPercent)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"safety_margin_percent": req.SafetyMarginPercent,
	})
}

// DTOs for new endpoints

type plantReadingDTO struct {
	ID                        int64              `json:"id"`
	PlantID                   string             `json:"plant_id"`
	TotalGeneration           float64            `json:"total_generation"`
	TotalContainerConsumption float64            `json:"total_container_consumption"`
	AvailablePower            float64            `json:"available_power"`
	GenerationSources         map[string]float64 `json:"generation_sources,omitempty"`
	ConsumptionSources        map[string]float64 `json:"consumption_sources,omitempty"`
	RecordedAt                string             `json:"recorded_at"`
}

func toPlantReadingDTO(reading database.PlantReading) plantReadingDTO {
	return plantReadingDTO{
		ID:                        reading.ID,
		PlantID:                   reading.PlantID,
		TotalGeneration:           reading.TotalGeneration,
		TotalContainerConsumption: reading.TotalContainerConsumption,
		AvailablePower:            reading.AvailablePower,
		GenerationSources:         reading.GenerationSources,
		ConsumptionSources:        reading.ConsumptionSources,
		RecordedAt:                formatTime(reading.RecordedAt),
	}
}

type powerBalanceEventDTO struct {
	ID                     int64    `json:"id"`
	MinerID                string   `json:"miner_id"`
	OldPreset              *string  `json:"old_preset"`
	NewPreset              *string  `json:"new_preset"`
	OldPower               *float64 `json:"old_power"`
	NewPower               *float64 `json:"new_power"`
	Reason                 string   `json:"reason"`
	TotalConsumptionBefore *float64 `json:"total_consumption_before"`
	TotalConsumptionAfter  *float64 `json:"total_consumption_after"`
	AvailablePower         *float64 `json:"available_power"`
	TargetPower            *float64 `json:"target_power"`
	Success                bool     `json:"success"`
	ErrorMessage           *string  `json:"error_message"`
	RecordedAt             string   `json:"recorded_at"`
}

func toPowerBalanceEventDTO(event database.PowerBalanceEvent) powerBalanceEventDTO {
	return powerBalanceEventDTO{
		ID:                     event.ID,
		MinerID:                event.MinerID,
		OldPreset:              event.OldPreset,
		NewPreset:              event.NewPreset,
		OldPower:               event.OldPower,
		NewPower:               event.NewPower,
		Reason:                 event.Reason,
		TotalConsumptionBefore: event.TotalConsumptionBefore,
		TotalConsumptionAfter:  event.TotalConsumptionAfter,
		AvailablePower:         event.AvailablePower,
		TargetPower:            event.TargetPower,
		Success:                event.Success,
		ErrorMessage:           event.ErrorMessage,
		RecordedAt:             formatTime(event.RecordedAt),
	}
}

type balanceStatusDTO struct {
	PlantGenerationKW    float64 `json:"plant_generation_kw"`
	PlantContainerKW     float64 `json:"plant_container_kw"`
	AvailablePowerKW     float64 `json:"available_power_kw"`
	SafetyMarginPercent  float64 `json:"safety_margin_percent"`
	TargetPowerKW        float64 `json:"target_power_kw"`
	TargetPowerW         float64 `json:"target_power_w"`
	CurrentConsumptionW  float64 `json:"current_consumption_w"`
	ManagedMinersCount   int     `json:"managed_miners_count"`
	Status               string  `json:"status"`
	LastReadingAt        *string `json:"last_reading_at,omitempty"`
}
