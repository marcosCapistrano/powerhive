package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// UpsertModel inserts a new model or updates an existing one matched by alias.
// When Presets is nil the stored presets remain untouched; an empty slice clears
// existing presets.
func (s *Store) UpsertModel(ctx context.Context, input ModelInput) (Model, error) {
	if err := validateModelInput(input); err != nil {
		return Model{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Model{}, fmt.Errorf("begin upsert model tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO models (name, alias, max_preset)
		VALUES (?, ?, ?)
		ON CONFLICT(alias) DO UPDATE SET
			name = excluded.name,
			max_preset = excluded.max_preset
	`, input.Name, input.Alias, nullableTrimmedString(input.MaxPreset)); err != nil {
		return Model{}, fmt.Errorf("upsert model %s: %w", input.Alias, err)
	}

	modelID, err := getModelIDByAlias(ctx, tx, input.Alias)
	if err != nil {
		return Model{}, err
	}

	if input.Presets != nil {
		if _, err := tx.ExecContext(ctx, `DELETE FROM model_presets WHERE model_id = ?`, modelID); err != nil {
			return Model{}, fmt.Errorf("clear presets for model %s: %w", input.Alias, err)
		}

		for idx, preset := range input.Presets {
			value := strings.TrimSpace(preset)
			if value == "" {
				return Model{}, fmt.Errorf("preset at position %d is empty", idx)
			}

			if _, err := tx.ExecContext(ctx, `
				INSERT INTO model_presets (model_id, value, position)
				VALUES (?, ?, ?)
			`, modelID, value, idx); err != nil {
				return Model{}, fmt.Errorf("insert preset %s for model %s: %w", value, input.Alias, err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return Model{}, fmt.Errorf("commit model tx: %w", err)
	}

	return s.getModelByID(ctx, modelID)
}

// GetModelByAlias fetches a model and its presets by alias.
func (s *Store) GetModelByAlias(ctx context.Context, alias string) (Model, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return Model{}, fmt.Errorf("begin read tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	modelID, err := getModelIDByAlias(ctx, tx, alias)
	if err != nil {
		return Model{}, err
	}

	model, err := s.getModelByIDTx(ctx, tx, modelID)
	if err != nil {
		return Model{}, err
	}

	if err := tx.Commit(); err != nil {
		return Model{}, fmt.Errorf("commit read tx: %w", err)
	}

	return model, nil
}

// ListModels returns every registered model ordered by alias.
func (s *Store) ListModels(ctx context.Context) ([]Model, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("begin list models tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx, `
		SELECT id, name, alias, max_preset, created_at
		FROM models
		ORDER BY alias
	`)
	if err != nil {
		return nil, fmt.Errorf("list models: %w", err)
	}
	defer rows.Close()

	var models []Model
	for rows.Next() {
		var (
			m   Model
			max sql.NullString
		)

		if err := rows.Scan(&m.ID, &m.Name, &m.Alias, &max, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan model: %w", err)
		}

		if max.Valid {
			m.MaxPreset = &max.String
		}

		presets, err := s.loadPresetsTx(ctx, tx, m.ID)
		if err != nil {
			return nil, err
		}
		m.Presets = presets
		models = append(models, m)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate models: %w", err)
	}

	return models, nil
}

func (s *Store) getModelByID(ctx context.Context, id int64) (Model, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return Model{}, fmt.Errorf("begin read tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	model, err := s.getModelByIDTx(ctx, tx, id)
	if err != nil {
		return Model{}, err
	}

	if err := tx.Commit(); err != nil {
		return Model{}, fmt.Errorf("commit read tx: %w", err)
	}

	return model, nil
}

func (s *Store) getModelByIDTx(ctx context.Context, tx *sql.Tx, id int64) (Model, error) {
	var (
		model Model
		max   sql.NullString
	)

	if err := tx.QueryRowContext(ctx, `
		SELECT id, name, alias, max_preset, created_at
		FROM models
		WHERE id = ?
	`, id).Scan(&model.ID, &model.Name, &model.Alias, &max, &model.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Model{}, fmt.Errorf("model %d not found", id)
		}
		return Model{}, fmt.Errorf("query model %d: %w", id, err)
	}

	if max.Valid {
		model.MaxPreset = &max.String
	}

	presets, err := s.loadPresetsTx(ctx, tx, id)
	if err != nil {
		return Model{}, err
	}
	model.Presets = presets

	return model, nil
}

func (s *Store) loadPresets(ctx context.Context, modelID int64) ([]string, error) {
	return s.loadPresetsTx(ctx, nil, modelID)
}

func (s *Store) loadPresetsTx(ctx context.Context, tx *sql.Tx, modelID int64) ([]string, error) {
	var (
		rows *sql.Rows
		err  error
	)

	query := `
		SELECT value
		FROM model_presets
		WHERE model_id = ?
		ORDER BY position, id
	`

	if tx != nil {
		rows, err = tx.QueryContext(ctx, query, modelID)
	} else {
		rows, err = s.db.QueryContext(ctx, query, modelID)
	}
	if err != nil {
		return nil, fmt.Errorf("load presets for model %d: %w", modelID, err)
	}
	defer rows.Close()

	var presets []string
	for rows.Next() {
		var preset string
		if err := rows.Scan(&preset); err != nil {
			return nil, fmt.Errorf("scan preset: %w", err)
		}
		presets = append(presets, preset)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate presets: %w", err)
	}

	return presets, nil
}

func getModelIDByAlias(ctx context.Context, tx *sql.Tx, alias string) (int64, error) {
	var id int64
	if err := tx.QueryRowContext(ctx, `
		SELECT id
		FROM models
		WHERE alias = ?
	`, alias).Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, fmt.Errorf("model alias %s not found", alias)
		}
		return 0, fmt.Errorf("query model alias %s: %w", alias, err)
	}
	return id, nil
}

func validateModelInput(input ModelInput) error {
	if strings.TrimSpace(input.Name) == "" {
		return fmt.Errorf("model name is required")
	}
	if strings.TrimSpace(input.Alias) == "" {
		return fmt.Errorf("model alias is required")
	}

	seen := make(map[string]struct{})
	for idx, preset := range input.Presets {
		value := strings.TrimSpace(preset)
		if value == "" {
			return fmt.Errorf("preset at position %d is empty", idx)
		}
		key := strings.ToLower(value)
		if _, exists := seen[key]; exists {
			return fmt.Errorf("duplicate preset value %s", value)
		}
		seen[key] = struct{}{}
	}

	return nil
}

// GetModelPresets returns all presets with power consumption data for a model.
func (s *Store) GetModelPresets(ctx context.Context, modelAlias string) ([]ModelPreset, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT mp.id, mp.model_id, mp.value, mp.position, mp.expected_power_w, mp.expected_hashrate_th, mp.created_at
		FROM model_presets mp
		JOIN models m ON mp.model_id = m.id
		WHERE m.alias = ?
		ORDER BY mp.position, mp.id
	`, modelAlias)
	if err != nil {
		return nil, fmt.Errorf("query model presets for %s: %w", modelAlias, err)
	}
	defer rows.Close()

	var presets []ModelPreset
	for rows.Next() {
		var (
			preset          ModelPreset
			expectedPower   sql.NullFloat64
			expectedHashrate sql.NullFloat64
		)

		if err := rows.Scan(&preset.ID, &preset.ModelID, &preset.Value, &preset.Position,
			&expectedPower, &expectedHashrate, &preset.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan preset: %w", err)
		}

		preset.ExpectedPowerW = floatPtrFromNull(expectedPower)
		preset.ExpectedHashrateTH = floatPtrFromNull(expectedHashrate)
		presets = append(presets, preset)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate presets: %w", err)
	}

	return presets, nil
}

// GetAllModelPresets returns all presets with power consumption data for all models,
// mapped by model alias. This is more efficient than calling GetModelPresets repeatedly.
func (s *Store) GetAllModelPresets(ctx context.Context) (map[string][]ModelPreset, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT m.alias, mp.id, mp.model_id, mp.value, mp.position, mp.expected_power_w, mp.expected_hashrate_th, mp.created_at
		FROM model_presets mp
		JOIN models m ON mp.model_id = m.id
		ORDER BY m.alias, mp.position, mp.id
	`)
	if err != nil {
		return nil, fmt.Errorf("query all model presets: %w", err)
	}
	defer rows.Close()

	result := make(map[string][]ModelPreset)
	for rows.Next() {
		var (
			alias            string
			preset           ModelPreset
			expectedPower    sql.NullFloat64
			expectedHashrate sql.NullFloat64
		)

		if err := rows.Scan(&alias, &preset.ID, &preset.ModelID, &preset.Value, &preset.Position,
			&expectedPower, &expectedHashrate, &preset.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan preset: %w", err)
		}

		preset.ExpectedPowerW = floatPtrFromNull(expectedPower)
		preset.ExpectedHashrateTH = floatPtrFromNull(expectedHashrate)
		result[alias] = append(result[alias], preset)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate all presets: %w", err)
	}

	return result, nil
}

// UpdatePresetPower updates the expected power consumption for a specific preset.
// Deprecated: Use UpdatePresetMetrics to update both power and hashrate.
func (s *Store) UpdatePresetPower(ctx context.Context, modelAlias, presetValue string, powerW float64) error {
	return s.UpdatePresetMetrics(ctx, modelAlias, presetValue, &powerW, nil)
}

// UpdatePresetMetrics updates the expected power consumption and/or hashrate for a specific preset.
// Pass nil for powerW or hashrateTH to skip updating that field.
func (s *Store) UpdatePresetMetrics(ctx context.Context, modelAlias, presetValue string, powerW, hashrateTH *float64) error {
	if powerW == nil && hashrateTH == nil {
		return fmt.Errorf("at least one of powerW or hashrateTH must be provided")
	}

	// Build dynamic query based on which fields are being updated
	var setClauses []string
	var args []interface{}

	if powerW != nil {
		setClauses = append(setClauses, "expected_power_w = ?")
		args = append(args, *powerW)
	}
	if hashrateTH != nil {
		setClauses = append(setClauses, "expected_hashrate_th = ?")
		args = append(args, *hashrateTH)
	}

	// Add WHERE clause parameters
	args = append(args, modelAlias, presetValue)

	query := fmt.Sprintf(`
		UPDATE model_presets
		SET %s
		WHERE model_id = (SELECT id FROM models WHERE alias = ?)
			AND value = ?
	`, strings.Join(setClauses, ", "))

	res, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update preset metrics for %s/%s: %w", modelAlias, presetValue, err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("preset %s not found for model %s", presetValue, modelAlias)
	}

	return nil
}
