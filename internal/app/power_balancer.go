package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
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

	// Get all managed miners with their current status
	miners, err := b.store.ListMiners(ctx)
	if err != nil {
		return fmt.Errorf("list miners: %w", err)
	}

	// Filter to managed miners that are online and have necessary data
	eligible := b.filterEligibleMiners(miners)
	if len(eligible) == 0 {
		b.log.Debug("no eligible miners for balancing")
		return nil
	}

	// Load preset power data by parsing preset values directly
	presetPowerMap, err := b.loadPresetPowerMap(eligible)
	if err != nil {
		return fmt.Errorf("load preset power data: %w", err)
	}

	// Calculate current total consumption from miners
	currentConsumption := b.calculateCurrentConsumption(eligible, presetPowerMap)

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

	// Adjust presets to match target
	adjustedCount := 0
	for _, me := range minerEfficiencies {
		// Check cooldown
		if lastChange, exists := cooldownMap[me.miner.ID]; exists {
			if time.Since(lastChange) < presetChangeCooldown {
				b.log.Debug("miner on cooldown", "miner", me.miner.ID)
				continue
			}
		}

		// Determine target preset
		targetPreset, targetPower, err := b.determineTargetPreset(me.miner, delta, presetPowerMap)
		if err != nil {
			b.log.Warn("failed to determine target preset", "miner", me.miner.ID, "err", err)
			continue
		}

		if targetPreset == nil || (me.currentPreset != nil && *targetPreset == *me.currentPreset) {
			continue // No change needed
		}

		// Apply preset change
		if err := b.applyPresetChange(ctx, me.miner, me.currentPreset, *targetPreset,
			me.currentPower, targetPower, currentConsumptionW, targetPowerW,
			plantReading.AvailablePower*1000, "automatic_balance"); err != nil {
			b.log.Error("failed to apply preset change", "miner", me.miner.ID, "err", err)
			continue
		}

		// Update cooldown map
		cooldownMap[me.miner.ID] = time.Now()
		adjustedCount++

		// Recalculate delta
		if me.currentPower != nil && targetPower != nil {
			powerChange := *targetPower - *me.currentPower
			delta -= powerChange
			currentConsumptionW += powerChange
		}

		b.log.Info("preset changed",
			"miner", me.miner.ID,
			"old_preset", stringOrNil(me.currentPreset),
			"new_preset", *targetPreset,
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
	presetPowerMap := make(map[string]map[string]float64)

	for _, miner := range miners {
		if miner.Model == nil {
			continue
		}

		modelAlias := miner.Model.Alias
		if _, exists := presetPowerMap[modelAlias]; exists {
			continue // Already loaded
		}

		// Parse preset values directly from the model's preset list
		powerMap := make(map[string]float64)
		for _, presetValue := range miner.Model.Presets {
			watts, err := parsePresetWattage(presetValue)
			if err != nil {
				b.log.Warn("failed to parse preset wattage, skipping",
					"model", modelAlias,
					"preset", presetValue,
					"err", err)
				continue
			}
			powerMap[presetValue] = watts
		}

		if len(powerMap) == 0 {
			b.log.Warn("no parseable presets found for model", "model", modelAlias)
		}

		presetPowerMap[modelAlias] = powerMap
	}

	return presetPowerMap, nil
}

func (b *PowerBalancer) calculateCurrentConsumption(miners []database.Miner, presetPowerMap map[string]map[string]float64) float64 {
	total := 0.0

	for _, miner := range miners {
		if miner.LatestStatus == nil || miner.LatestStatus.Preset == nil {
			continue
		}
		if miner.Model == nil {
			continue
		}

		preset := *miner.LatestStatus.Preset
		modelAlias := miner.Model.Alias

		if powerMap, exists := presetPowerMap[modelAlias]; exists {
			if power, exists := powerMap[preset]; exists {
				total += power
			}
		}
	}

	return total
}

func (b *PowerBalancer) calculateEfficiencies(miners []database.Miner, presetPowerMap map[string]map[string]float64) []minerEfficiency {
	var efficiencies []minerEfficiency

	for _, miner := range miners {
		if miner.LatestStatus == nil || miner.LatestStatus.Hashrate == nil {
			continue
		}
		if miner.Model == nil {
			continue
		}

		hashrateTH := *miner.LatestStatus.Hashrate / 1e12 // Convert to TH/s
		if hashrateTH <= 0 {
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

		if currentPower == nil || *currentPower <= 0 {
			continue
		}

		efficiency := *currentPower / hashrateTH

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
