package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"powerhive/internal/config"
	"powerhive/internal/database"
	"powerhive/internal/firmware"
)

const (
	// Minimum time between preset changes for a single miner to avoid thrashing
	presetChangeCooldown   = 30 * time.Second
	balancerRequestTimeout = 5 * time.Second
)

// PowerBalancer orchestrates power consumption across miners to match available generation.
type PowerBalancer struct {
	store    *database.Store
	cfg      config.AppConfig
	log      *slog.Logger
	interval time.Duration
}

// NewPowerBalancer creates a new power balancing orchestrator.
func NewPowerBalancer(store *database.Store, cfg config.AppConfig, logger *slog.Logger) *PowerBalancer {
	return &PowerBalancer{
		store:    store,
		cfg:      cfg,
		log:      logger.With("component", "balancer"),
		interval: time.Duration(cfg.Intervals.BalancerSeconds) * time.Second,
	}
}

// Run starts the power balancing loop.
func (b *PowerBalancer) Run(ctx context.Context) {
	b.log.Info("starting power balancing loop", "interval", b.interval)

	// Initial run after a short delay to let other services populate data
	time.Sleep(5 * time.Second)
	if err := b.balance(ctx); err != nil {
		b.log.Error("initial balance failed", "err", err)
	}

	ticker := time.NewTicker(b.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			b.log.Info("stopping power balancing loop", "reason", ctx.Err())
			return
		case <-ticker.C:
			if err := b.balance(ctx); err != nil {
				b.log.Error("balance cycle failed", "err", err)
			}
		}
	}
}

func (b *PowerBalancer) balance(ctx context.Context) error {
	// Get latest plant reading
	plantReading, err := b.store.GetLatestPlantReading(ctx)
	if err != nil {
		return fmt.Errorf("get plant reading: %w", err)
	}
	if plantReading == nil {
		b.log.Warn("no plant readings available yet, skipping balance")
		return nil
	}

	// Get safety margin from settings
	safetyMarginStr, err := b.store.GetAppSetting(ctx, "safety_margin_percent")
	if err != nil {
		return fmt.Errorf("get safety margin: %w", err)
	}

	var safetyMargin float64
	if err := json.Unmarshal([]byte(safetyMarginStr), &safetyMargin); err != nil {
		safetyMargin = 10.0 // Default fallback
	}

	// Calculate target power (plant generation minus safety margin)
	targetPower := plantReading.TotalGeneration * (1.0 - safetyMargin/100.0)

	b.log.Debug("balance cycle starting",
		"available_kw", plantReading.AvailablePower,
		"safety_margin_pct", safetyMargin,
		"target_kw", targetPower,
	)

	// Get all miners with their current status
	miners, err := b.store.ListMiners(ctx)
	if err != nil {
		return fmt.Errorf("list miners: %w", err)
	}

	// Filter to managed miners for balancing decisions
	eligible := b.filterEligibleMiners(miners)
	if len(eligible) == 0 {
		b.log.Debug("no eligible miners for balancing")
	}

	// Get all online miners (managed + unmanaged) for consumption calculation
	allOnline := b.filterOnlineMiners(miners)

	// Load preset power data for all online miners
	presetPowerMap, err := b.loadPresetPowerMap(allOnline)
	if err != nil {
		return fmt.Errorf("load preset power data: %w", err)
	}

	// Calculate current total consumption from ALL online miners (managed + unmanaged)
	currentConsumption := b.calculateCurrentConsumption(allOnline, presetPowerMap)

	// Convert target from kW to W for comparison with miner presets
	targetPowerW := targetPower * 1000.0
	currentConsumptionW := currentConsumption

	b.log.Info("power status",
		"current_w", currentConsumptionW,
		"target_w", targetPowerW,
		"delta_w", targetPowerW-currentConsumptionW,
		"eligible_miners", len(eligible),
	)

	// Decide if we need to adjust
	delta := targetPowerW - currentConsumptionW
	if math.Abs(delta) < 2000 { // Within 2000W tolerance (roughly one miner's consumption)
		b.log.Debug("consumption within tolerance, no changes needed")
		return nil
	}

	// Sort miners by efficiency (W/TH) - worst first for reduction, best first for increase
	minerEfficiencies := b.calculateEfficiencies(eligible, presetPowerMap)
	if delta < 0 {
		// Need to reduce consumption - adjust least efficient miners first
		sort.Slice(minerEfficiencies, func(i, j int) bool {
			return minerEfficiencies[i].efficiency > minerEfficiencies[j].efficiency
		})
		b.log.Info("reducing consumption", "miners_to_adjust", len(minerEfficiencies))
	} else {
		// Can increase consumption - increase most efficient miners first
		sort.Slice(minerEfficiencies, func(i, j int) bool {
			return minerEfficiencies[i].efficiency < minerEfficiencies[j].efficiency
		})
		b.log.Info("increasing consumption", "miners_to_adjust", len(minerEfficiencies))
	}

	// Load cooldown map
	cooldownMap, err := b.loadCooldownMap(ctx)
	if err != nil {
		b.log.Warn("failed to load cooldown map, continuing", "err", err)
		cooldownMap = make(map[string]time.Time)
	}

	// Calculate planned changes and expected consumption
	plannedChanges := make(map[string]struct {
		targetPreset *string
		targetPower  *float64
	})

	expectedConsumption := currentConsumptionW
	for _, me := range minerEfficiencies {
		// Check cooldown
		if lastChange, exists := cooldownMap[me.miner.ID]; exists {
			if time.Since(lastChange) < presetChangeCooldown {
				continue
			}
		}

		// Determine target preset
		targetPreset, targetPower, err := b.determineTargetPreset(me.miner, delta, presetPowerMap)
		if err != nil {
			continue
		}

		if targetPreset == nil || (me.currentPreset != nil && *targetPreset == *me.currentPreset) {
			continue // No change needed
		}

		// Store planned change
		plannedChanges[me.miner.ID] = struct {
			targetPreset *string
			targetPower  *float64
		}{targetPreset: targetPreset, targetPower: targetPower}

		// Calculate expected consumption (current of unchanged + new of changed)
		if me.currentPower != nil && targetPower != nil {
			powerChange := *targetPower - *me.currentPower
			expectedConsumption += powerChange
			delta -= powerChange
		}

		// Stop planning if we're close enough to target
		if math.Abs(delta) < 2000 {
			break
		}
	}

	// Store and POST expected consumption
	if err := b.storeExpectedConsumption(ctx, expectedConsumption); err != nil {
		b.log.Warn("failed to store expected consumption", "err", err)
	}

	if err := b.postExpectedConsumptionToTestServer(ctx, expectedConsumption); err != nil {
		b.log.Warn("failed to post expected consumption to test server", "err", err)
	}

	b.log.Info("expected consumption calculated",
		"current_w", currentConsumptionW,
		"expected_w", expectedConsumption,
		"delta_w", expectedConsumption-currentConsumptionW,
		"planned_changes", len(plannedChanges))

	// Now apply the planned changes
	adjustedCount := 0
	delta = targetPowerW - currentConsumptionW // Reset delta for actual application

	for _, me := range minerEfficiencies {
		// Check if we have a planned change for this miner
		planned, exists := plannedChanges[me.miner.ID]
		if !exists {
			continue
		}

		// Apply preset change
		if err := b.applyPresetChange(ctx, me.miner, me.currentPreset, *planned.targetPreset,
			me.currentPower, planned.targetPower, currentConsumptionW, targetPowerW,
			plantReading.AvailablePower*1000, "automatic_balance"); err != nil {
			b.log.Error("failed to apply preset change", "miner", me.miner.ID, "err", err)
			continue
		}

		// Update cooldown map
		cooldownMap[me.miner.ID] = time.Now()
		adjustedCount++

		// Recalculate delta
		if me.currentPower != nil && planned.targetPower != nil {
			powerChange := *planned.targetPower - *me.currentPower
			delta -= powerChange
			currentConsumptionW += powerChange
		}

		b.log.Info("preset changed",
			"miner", me.miner.ID,
			"old_preset", stringOrNil(me.currentPreset),
			"new_preset", *planned.targetPreset,
			"delta_remaining_w", delta,
		)

		// Stop if we're close enough to target
		if math.Abs(delta) < 2000 {
			break
		}
	}

	// Save cooldown map
	if err := b.saveCooldownMap(ctx, cooldownMap); err != nil {
		b.log.Warn("failed to save cooldown map", "err", err)
	}

	if adjustedCount > 0 {
		b.log.Info("balance cycle complete", "miners_adjusted", adjustedCount)
	}

	return nil
}

type minerEfficiency struct {
	miner         database.Miner
	efficiency    float64 // W/TH
	currentPreset *string
	currentPower  *float64
}

func (b *PowerBalancer) filterEligibleMiners(miners []database.Miner) []database.Miner {
	var eligible []database.Miner
	for _, miner := range miners {
		if !miner.Managed {
			continue
		}
		if miner.IP == nil || *miner.IP == "" {
			continue
		}
		if miner.APIKey == nil || *miner.APIKey == "" {
			continue
		}
		if miner.Model == nil {
			continue
		}
		eligible = append(eligible, miner)
	}
	return eligible
}

// filterOnlineMiners returns all miners (managed and unmanaged) that are online
// and have a model. Used for calculating total consumption baseline.
func (b *PowerBalancer) filterOnlineMiners(miners []database.Miner) []database.Miner {
	var online []database.Miner
	for _, miner := range miners {
		if miner.IP == nil || *miner.IP == "" {
			continue
		}
		if miner.Model == nil {
			continue
		}
		online = append(online, miner)
	}
	return online
}

// parsePresetWattage extracts wattage from preset strings like "900W", "1000W", "1200W".
// Returns the power in watts and an error if the format is invalid.
func parsePresetWattage(preset string) (float64, error) {
	// Trim whitespace and convert to lowercase for consistent parsing
	preset = strings.ToLower(strings.TrimSpace(preset))

	if preset == "" {
		return 0, fmt.Errorf("empty preset value")
	}

	// Skip non-power presets
	if preset == "disabled" {
		return 0, fmt.Errorf("preset is 'disabled', skipping")
	}

	// Handle two formats:
	// 1. S21 format: "3010W", "3420W" (with 'W' suffix)
	// 2. S19 format: "1100", "1300" (just numbers representing watts)
	// TrimSuffix only removes if present, safe to call unconditionally
	preset = strings.TrimSuffix(preset, "w")

	// Parse the numeric value
	watts, err := strconv.ParseFloat(preset, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse wattage from preset '%s': %w", preset, err)
	}

	if watts <= 0 {
		return 0, fmt.Errorf("invalid wattage value (must be positive): %.2f", watts)
	}

	return watts, nil
}

func (b *PowerBalancer) loadPresetPowerMap(miners []database.Miner) (map[string]map[string]float64, error) {
	// Load all preset power data from database
	allPresets, err := b.store.GetAllModelPresets(context.Background())
	if err != nil {
		return nil, fmt.Errorf("load model presets: %w", err)
	}

	presetPowerMap := make(map[string]map[string]float64)

	// Convert database presets to power map
	for modelAlias, presets := range allPresets {
		powerMap := make(map[string]float64)
		for _, preset := range presets {
			// Only include presets with expected power set
			if preset.ExpectedPowerW != nil {
				powerMap[preset.Value] = *preset.ExpectedPowerW
			}
		}

		// If no presets have power set, try parsing from preset names as fallback
		if len(powerMap) == 0 {
			// Find the model to get preset list
			for _, miner := range miners {
				if miner.Model != nil && miner.Model.Alias == modelAlias {
					for _, presetValue := range miner.Model.Presets {
						watts, err := parsePresetWattage(presetValue)
						if err != nil {
							// Skip presets that can't be parsed (like "disabled")
							continue
						}
						powerMap[presetValue] = watts
					}
					break
				}
			}
		}

		if len(powerMap) > 0 {
			presetPowerMap[modelAlias] = powerMap
		} else {
			b.log.Warn("no preset power data found for model", "model", modelAlias)
		}
	}

	return presetPowerMap, nil
}

func (b *PowerBalancer) calculateCurrentConsumption(miners []database.Miner, presetPowerMap map[string]map[string]float64) float64 {
	total := 0.0

	for _, miner := range miners {
		if miner.Model == nil {
			continue
		}

		var minerPower float64
		found := false

		// Try to get power from preset map (using current preset)
		if miner.LatestStatus != nil && miner.LatestStatus.Preset != nil {
			preset := *miner.LatestStatus.Preset
			modelAlias := miner.Model.Alias

			if powerMap, exists := presetPowerMap[modelAlias]; exists {
				if power, exists := powerMap[preset]; exists {
					minerPower = power
					found = true
				}
			}
		}

		// Fallback to status power consumption if available
		if !found && miner.LatestStatus != nil && miner.LatestStatus.PowerConsumption != nil {
			minerPower = *miner.LatestStatus.PowerConsumption
			found = true
		}

		// If still not found, log warning but don't add to total
		if !found {
			if miner.LatestStatus != nil {
				b.log.Debug("no power data for miner",
					"miner", miner.ID,
					"preset", stringOrNil(miner.LatestStatus.Preset))
			} else {
				b.log.Debug("no power data for miner (no status)",
					"miner", miner.ID)
			}
		} else {
			total += minerPower
		}
	}

	return total
}

func (b *PowerBalancer) calculateEfficiencies(miners []database.Miner, presetPowerMap map[string]map[string]float64) []minerEfficiency {
	var efficiencies []minerEfficiency

	for _, miner := range miners {
		if miner.LatestStatus == nil {
			continue
		}
		if miner.Model == nil {
			continue
		}

		var currentPower *float64
		if miner.LatestStatus.Preset != nil {
			if powerMap, exists := presetPowerMap[miner.Model.Alias]; exists {
				if power, exists := powerMap[*miner.LatestStatus.Preset]; exists {
					currentPower = &power
				}
			}
		}

		// Fallback to power_consumption from status if available
		if currentPower == nil && miner.LatestStatus.PowerConsumption != nil {
			currentPower = miner.LatestStatus.PowerConsumption
		}

		// Skip if no power data available
		if currentPower == nil {
			continue
		}

		// Calculate hashrate in TH/s
		var hashrateTH float64
		if miner.LatestStatus.Hashrate != nil {
			hashrateTH = *miner.LatestStatus.Hashrate / 1e12
		}

		// For miners with 0 hashrate (like disabled preset), set very high efficiency
		// so they are prioritized for increases (sorted worst first)
		var efficiency float64
		if hashrateTH <= 0 {
			// Very high efficiency means "worst" for reduction, "best" for increase
			// Use a large number so disabled miners are increased first
			efficiency = 1e9 // Essentially infinite W/TH
		} else {
			efficiency = *currentPower / hashrateTH
		}

		efficiencies = append(efficiencies, minerEfficiency{
			miner:         miner,
			efficiency:    efficiency,
			currentPreset: miner.LatestStatus.Preset,
			currentPower:  currentPower,
		})
	}

	return efficiencies
}

func (b *PowerBalancer) determineTargetPreset(miner database.Miner, delta float64, presetPowerMap map[string]map[string]float64) (*string, *float64, error) {
	if miner.Model == nil {
		return nil, nil, fmt.Errorf("miner has no model")
	}

	powerMap, exists := presetPowerMap[miner.Model.Alias]
	if !exists || len(powerMap) == 0 {
		return nil, nil, fmt.Errorf("no preset power data for model %s", miner.Model.Alias)
	}

	// Build sorted list of presets by power
	type presetPower struct {
		preset string
		power  float64
	}
	var presets []presetPower
	for preset, power := range powerMap {
		presets = append(presets, presetPower{preset: preset, power: power})
	}
	sort.Slice(presets, func(i, j int) bool {
		return presets[i].power < presets[j].power
	})

	// Get current preset power
	var currentPower *float64
	if miner.LatestStatus != nil && miner.LatestStatus.Preset != nil {
		if power, exists := powerMap[*miner.LatestStatus.Preset]; exists {
			currentPower = &power
		}
	}

	// Determine direction
	needsReduction := delta < 0

	// Find target preset
	var targetPreset *string
	var targetPower *float64

	if needsReduction {
		// Need to reduce - find a lower preset
		for i := len(presets) - 1; i >= 0; i-- {
			if currentPower == nil || presets[i].power < *currentPower {
				preset := presets[i].preset
				power := presets[i].power
				targetPreset = &preset
				targetPower = &power
				break
			}
		}
	} else {
		// Can increase - find a higher preset (but respect max_preset)
		maxPreset := miner.Model.MaxPreset
		for i := 0; i < len(presets); i++ {
			if currentPower == nil || presets[i].power > *currentPower {
				// Check if this exceeds max_preset
				if maxPreset != nil && presets[i].preset != *maxPreset {
					// Check if we've passed max_preset
					foundMax := false
					for j := 0; j <= i; j++ {
						if presets[j].preset == *maxPreset {
							foundMax = true
							break
						}
					}
					if !foundMax {
						continue // Haven't reached max yet
					}
					// If current > max, don't increase
					if currentPower != nil {
						for _, pp := range presets {
							if miner.LatestStatus.Preset != nil && pp.preset == *miner.LatestStatus.Preset {
								if pp.power >= presets[i].power {
									break // Current is already >= max
								}
							}
						}
					}
				}

				// Verify not exceeding max_preset
				if maxPreset != nil {
					exceeds := false
					for _, pp := range presets {
						if pp.preset == *maxPreset && presets[i].power > pp.power {
							exceeds = true
							break
						}
					}
					if exceeds {
						continue
					}
				}

				preset := presets[i].preset
				power := presets[i].power
				targetPreset = &preset
				targetPower = &power
				break
			}
		}
	}

	return targetPreset, targetPower, nil
}

func (b *PowerBalancer) applyPresetChange(ctx context.Context, miner database.Miner, oldPreset *string, newPreset string, oldPower, newPower *float64, totalConsumBefore, targetPower, availablePower float64, reason string) error {
	if miner.IP == nil || miner.APIKey == nil {
		return fmt.Errorf("miner missing IP or API key")
	}

	client, err := firmware.NewClient(*miner.IP)
	if err != nil {
		return fmt.Errorf("create firmware client: %w", err)
	}

	reqCtx, cancel := context.WithTimeout(ctx, balancerRequestTimeout)
	defer cancel()

	// Apply preset change via firmware API
	result, err := client.SetPreset(reqCtx, *miner.APIKey, newPreset)
	if err != nil {
		// Log failure event
		_, _ = b.store.RecordPowerBalanceEvent(ctx, database.PowerBalanceEventInput{
			MinerID:                miner.ID,
			OldPreset:              oldPreset,
			NewPreset:              &newPreset,
			OldPower:               oldPower,
			NewPower:               newPower,
			Reason:                 reason,
			TotalConsumptionBefore: &totalConsumBefore,
			AvailablePower:         &availablePower,
			TargetPower:            &targetPower,
			Success:                false,
			ErrorMessage:           ptrString(err.Error()),
			RecordedAt:             time.Now().UTC(),
		})
		return fmt.Errorf("set preset via firmware: %w", err)
	}

	// Log if restart/reboot is required
	if result != nil {
		if result.RestartRequired {
			b.log.Info("miner restart required after preset change", "miner", miner.ID, "preset", newPreset)
			client.RestartMining(reqCtx, *miner.APIKey)
		}
		if result.RebootRequired {
			b.log.Info("miner reboot required after preset change", "miner", miner.ID, "preset", newPreset)
		}
	}

	// Calculate new total consumption
	var powerChange float64
	if oldPower != nil && newPower != nil {
		powerChange = *newPower - *oldPower
	}
	totalConsumAfter := totalConsumBefore + powerChange

	// Log success event
	if _, err := b.store.RecordPowerBalanceEvent(ctx, database.PowerBalanceEventInput{
		MinerID:                miner.ID,
		OldPreset:              oldPreset,
		NewPreset:              &newPreset,
		OldPower:               oldPower,
		NewPower:               newPower,
		Reason:                 reason,
		TotalConsumptionBefore: &totalConsumBefore,
		TotalConsumptionAfter:  &totalConsumAfter,
		AvailablePower:         &availablePower,
		TargetPower:            &targetPower,
		Success:                true,
		RecordedAt:             time.Now().UTC(),
	}); err != nil {
		b.log.Warn("failed to log balance event", "err", err)
	}

	return nil
}

func (b *PowerBalancer) loadCooldownMap(ctx context.Context) (map[string]time.Time, error) {
	data, err := b.store.GetAppSetting(ctx, "last_preset_change")
	if err != nil {
		return make(map[string]time.Time), nil
	}

	var cooldownMap map[string]time.Time
	if err := json.Unmarshal([]byte(data), &cooldownMap); err != nil {
		return make(map[string]time.Time), nil
	}

	return cooldownMap, nil
}

func (b *PowerBalancer) saveCooldownMap(ctx context.Context, cooldownMap map[string]time.Time) error {
	data, err := json.Marshal(cooldownMap)
	if err != nil {
		return err
	}

	return b.store.SetAppSetting(ctx, "last_preset_change", string(data))
}

func stringOrNil(s *string) string {
	if s == nil {
		return "nil"
	}
	return *s
}

func ptrString(s string) *string {
	return &s
}

// storeExpectedConsumption stores the expected consumption value in app settings.
func (b *PowerBalancer) storeExpectedConsumption(ctx context.Context, consumptionW float64) error {
	data, err := json.Marshal(consumptionW)
	if err != nil {
		return err
	}
	return b.store.SetAppSetting(ctx, "expected_consumption_w", string(data))
}

// GetExpectedConsumption retrieves the stored expected consumption value.
func (b *PowerBalancer) GetExpectedConsumption(ctx context.Context) (float64, error) {
	data, err := b.store.GetAppSetting(ctx, "expected_consumption_w")
	if err != nil {
		return 0, err
	}

	var consumptionW float64
	if err := json.Unmarshal([]byte(data), &consumptionW); err != nil {
		return 0, err
	}
	return consumptionW, nil
}

// postExpectedConsumptionToTestServer sends the expected consumption to the test server.
func (b *PowerBalancer) postExpectedConsumptionToTestServer(ctx context.Context, consumptionW float64) error {
	if !b.cfg.Plant.TestMode {
		return nil // Skip if not in test mode
	}

	if b.cfg.Plant.TestServerURL == "" {
		return fmt.Errorf("test mode enabled but test server URL not configured")
	}

	// Convert watts to megawatts
	consumptionMW := consumptionW / 1_000_000.0

	payload := map[string]float64{
		"expected_consumption_mw": consumptionMW,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	url := b.cfg.Plant.TestServerURL + "/data/consumption"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payloadBytes))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("post to test server: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("test server returned status %d", resp.StatusCode)
	}

	b.log.Info("posted expected consumption to test server",
		"consumption_w", consumptionW,
		"consumption_mw", consumptionMW,
		"url", url)

	return nil
}
