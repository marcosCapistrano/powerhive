package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// SaveMinerSettings stores a new settings snapshot and associates it with the miner.
func (s *Store) SaveMinerSettings(ctx context.Context, minerID string, input SettingsInput) (Settings, error) {
	minerID = strings.TrimSpace(minerID)
	if minerID == "" {
		return Settings{}, fmt.Errorf("miner id is required")
	}

	if err := validateSettingsInput(input); err != nil {
		return Settings{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Settings{}, fmt.Errorf("begin save settings tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var (
		fanMinCount          any = nullableInt(input.Cooling.FanMinCount)
		fanMinDuty           any = nullableInt(input.Cooling.FanMinDuty)
		fanMaxDuty           any = nullableInt(input.Cooling.FanMaxDuty)
		coolingMode              = strings.TrimSpace(input.Cooling.Mode)
		minOperationalChains any = nullableInt(input.Misc.MinOperationalChains)
		preset               any = nullableTrimmedString(input.Preset)
	)

	if coolingMode == "" {
		coolingMode = "auto"
	}

	res, err := tx.ExecContext(ctx, `
		INSERT INTO settings (
			fan_min_count,
			fan_min_duty,
			fan_max_duty,
			cooling_mode,
			ignore_broken_sensors,
			min_operational_chains,
			preset
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`, fanMinCount, fanMinDuty, fanMaxDuty, coolingMode, boolToInt(input.Misc.IgnoreBrokenSensors), minOperationalChains, preset)
	if err != nil {
		return Settings{}, fmt.Errorf("insert settings: %w", err)
	}

	settingsID, err := res.LastInsertId()
	if err != nil {
		return Settings{}, fmt.Errorf("read settings id: %w", err)
	}

	for idx, pool := range input.Pools {
		if strings.TrimSpace(pool.URL) == "" {
			return Settings{}, fmt.Errorf("pool at position %d requires a url", idx)
		}

		if _, err := tx.ExecContext(ctx, `
			INSERT INTO settings_pools (settings_id, position, url, username, password)
			VALUES (?, ?, ?, ?, ?)
		`, settingsID, idx, strings.TrimSpace(pool.URL), nullableTrimmedString(pool.Username), nullableTrimmedString(pool.Password)); err != nil {
			return Settings{}, fmt.Errorf("insert pool %s: %w", pool.URL, err)
		}
	}

	result, err := tx.ExecContext(ctx, `
		UPDATE miners
		SET settings_id = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, settingsID, minerID)
	if err != nil {
		return Settings{}, fmt.Errorf("attach settings to miner %s: %w", minerID, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return Settings{}, fmt.Errorf("settings attach rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return Settings{}, fmt.Errorf("miner %s not found", minerID)
	}

	if err := tx.Commit(); err != nil {
		return Settings{}, fmt.Errorf("commit settings tx: %w", err)
	}

	return s.GetSettingsByID(ctx, settingsID)
}

// GetSettingsByID fetches a settings snapshot along with its pools.
func (s *Store) GetSettingsByID(ctx context.Context, settingsID int64) (Settings, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return Settings{}, fmt.Errorf("begin get settings tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	settings, err := s.getSettingsByIDTx(ctx, tx, settingsID)
	if err != nil {
		return Settings{}, err
	}

	if err := tx.Commit(); err != nil {
		return Settings{}, fmt.Errorf("commit get settings tx: %w", err)
	}

	return settings, nil
}

func (s *Store) getSettingsByIDTx(ctx context.Context, tx *sql.Tx, settingsID int64) (Settings, error) {
	var (
		settings             Settings
		fanMinCount          sql.NullInt64
		fanMinDuty           sql.NullInt64
		fanMaxDuty           sql.NullInt64
		minOperationalChains sql.NullInt64
		preset               sql.NullString
	)

	err := tx.QueryRowContext(ctx, `
		SELECT id, fan_min_count, fan_min_duty, fan_max_duty, cooling_mode,
		       ignore_broken_sensors, min_operational_chains, preset, created_at
		FROM settings
		WHERE id = ?
	`, settingsID).Scan(
		&settings.ID,
		&fanMinCount,
		&fanMinDuty,
		&fanMaxDuty,
		&settings.Cooling.Mode,
		&settings.Misc.IgnoreBrokenSensors,
		&minOperationalChains,
		&preset,
		&settings.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Settings{}, fmt.Errorf("settings %d not found", settingsID)
		}
		return Settings{}, fmt.Errorf("query settings %d: %w", settingsID, err)
	}

	settings.Cooling.FanMinCount = intPtrFromNull(fanMinCount)
	settings.Cooling.FanMinDuty = intPtrFromNull(fanMinDuty)
	settings.Cooling.FanMaxDuty = intPtrFromNull(fanMaxDuty)
	settings.Misc.MinOperationalChains = intPtrFromNull(minOperationalChains)
	if preset.Valid {
		value := strings.TrimSpace(preset.String)
		settings.Preset = &value
	}

	pools, err := loadPools(ctx, tx, settings.ID)
	if err != nil {
		return Settings{}, err
	}
	settings.Pools = pools

	return settings, nil
}

func loadPools(ctx context.Context, tx *sql.Tx, settingsID int64) ([]Pool, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, position, url, username, password
		FROM settings_pools
		WHERE settings_id = ?
		ORDER BY position, id
	`, settingsID)
	if err != nil {
		return nil, fmt.Errorf("query pools for settings %d: %w", settingsID, err)
	}
	defer rows.Close()

	var pools []Pool
	for rows.Next() {
		var (
			pool     Pool
			username sql.NullString
			password sql.NullString
		)

		if err := rows.Scan(&pool.ID, &pool.Position, &pool.URL, &username, &password); err != nil {
			return nil, fmt.Errorf("scan pool: %w", err)
		}

		pool.SettingsID = settingsID
		pool.Username = stringPtrFromNull(username)
		pool.Password = stringPtrFromNull(password)
		pools = append(pools, pool)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pools: %w", err)
	}

	return pools, nil
}

func validateSettingsInput(input SettingsInput) error {
	mode := strings.TrimSpace(input.Cooling.Mode)
	if mode == "" {
		mode = "auto"
	}

	switch strings.ToLower(mode) {
	case "auto", "manual", "immersion":
	default:
		return fmt.Errorf("invalid cooling mode %q", input.Cooling.Mode)
	}

	for idx, pool := range input.Pools {
		if strings.TrimSpace(pool.URL) == "" {
			return fmt.Errorf("pool at position %d requires a url", idx)
		}
	}

	return nil
}
