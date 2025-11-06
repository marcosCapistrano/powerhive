package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// UpsertMiner ensures a miner row exists and applies the provided updates.
func (s *Store) UpsertMiner(ctx context.Context, params UpsertMinerParams) (Miner, error) {
	minerID := strings.TrimSpace(params.ID)
	if minerID == "" {
		return Miner{}, fmt.Errorf("miner id is required")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Miner{}, fmt.Errorf("begin upsert miner tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `INSERT INTO miners (id) VALUES (?) ON CONFLICT(id) DO NOTHING`, minerID); err != nil {
		return Miner{}, fmt.Errorf("ensure miner %s: %w", minerID, err)
	}

	var (
		sets []string
		args []any
	)

	if params.IP != nil {
		ip := strings.TrimSpace(*params.IP)
		if ip == "" {
			sets = append(sets, "ip = NULL")
		} else {
			sets = append(sets, "ip = ?")
			args = append(args, ip)
		}
	}

	if params.APIKey != nil {
		apiKey := strings.TrimSpace(*params.APIKey)
		if apiKey == "" {
			sets = append(sets, "api_key = NULL")
		} else {
			sets = append(sets, "api_key = ?")
			args = append(args, apiKey)
		}
	}

	if params.Managed != nil {
		sets = append(sets, "managed = ?")
		args = append(args, boolToInt(*params.Managed))
	}

	if params.UnlockPass != nil {
		pass := strings.TrimSpace(*params.UnlockPass)
		if pass == "" {
			return Miner{}, fmt.Errorf("unlock password cannot be empty")
		}
		sets = append(sets, "unlock_pass = ?")
		args = append(args, pass)
	}

	if params.ModelAlias != nil {
		alias := strings.TrimSpace(*params.ModelAlias)
		if alias == "" {
			return Miner{}, fmt.Errorf("model alias cannot be empty")
		}

		modelID, err := getModelIDByAlias(ctx, tx, alias)
		if err != nil {
			return Miner{}, err
		}
		sets = append(sets, "model_id = ?")
		args = append(args, modelID)
	}

	if len(sets) > 0 {
		sets = append(sets, "updated_at = CURRENT_TIMESTAMP")
		query := fmt.Sprintf("UPDATE miners SET %s WHERE id = ?", strings.Join(sets, ", "))
		args = append(args, minerID)

		if _, err := tx.ExecContext(ctx, query, args...); err != nil {
			return Miner{}, fmt.Errorf("update miner %s: %w", minerID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return Miner{}, fmt.Errorf("commit miner tx: %w", err)
	}

	return s.GetMiner(ctx, minerID)
}

// GetMiner fetches a miner and eagerly loads related data.
func (s *Store) GetMiner(ctx context.Context, minerID string) (Miner, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return Miner{}, fmt.Errorf("begin get miner tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var (
		miner          Miner
		ip             sql.NullString
		apiKey         sql.NullString
		modelID        sql.NullInt64
		settingsID     sql.NullInt64
		latestStatusID sql.NullInt64
		managedInt     int
		unlockPass     string
	)

	err = tx.QueryRowContext(ctx, `
		SELECT id, ip, api_key, managed, unlock_pass, model_id, settings_id, latest_status_id, created_at, updated_at
		FROM miners
		WHERE id = ?
	`, minerID).Scan(&miner.ID, &ip, &apiKey, &managedInt, &unlockPass, &modelID, &settingsID, &latestStatusID, &miner.CreatedAt, &miner.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Miner{}, fmt.Errorf("miner %s not found", minerID)
		}
		return Miner{}, fmt.Errorf("query miner %s: %w", minerID, err)
	}

	if ip.Valid {
		miner.IP = &ip.String
	}
	if apiKey.Valid {
		miner.APIKey = &apiKey.String
	}
	miner.Managed = managedInt != 0
	miner.UnlockPass = unlockPass

	if modelID.Valid {
		model, err := s.getModelByIDTx(ctx, tx, modelID.Int64)
		if err != nil {
			return Miner{}, err
		}
		miner.Model = &model
	}

	if settingsID.Valid {
		settings, err := s.getSettingsByIDTx(ctx, tx, settingsID.Int64)
		if err != nil {
			return Miner{}, err
		}
		miner.Settings = &settings
	}

	if latestStatusID.Valid {
		status, err := s.getStatusByIDTx(ctx, tx, latestStatusID.Int64)
		if err != nil {
			return Miner{}, err
		}
		miner.LatestStatus = &status
		miner.LatestStatusID = &status.ID
	}

	if err := tx.Commit(); err != nil {
		return Miner{}, fmt.Errorf("commit get miner tx: %w", err)
	}

	return miner, nil
}

// ListMiners returns every miner without loading heavy historical fields.
func (s *Store) ListMiners(ctx context.Context) ([]Miner, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("begin list miners tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx, `
		SELECT id, ip, api_key, managed, unlock_pass, model_id, settings_id, latest_status_id, created_at, updated_at
		FROM miners
		ORDER BY id
	`)
	if err != nil {
		return nil, fmt.Errorf("list miners: %w", err)
	}
	defer rows.Close()

	var miners []Miner

	for rows.Next() {
		var (
			miner          Miner
			ip             sql.NullString
			apiKey         sql.NullString
			modelID        sql.NullInt64
			settingsID     sql.NullInt64
			latestStatusID sql.NullInt64
			managedInt     int
		)

		if err := rows.Scan(&miner.ID, &ip, &apiKey, &managedInt, &miner.UnlockPass, &modelID, &settingsID, &latestStatusID, &miner.CreatedAt, &miner.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan miner: %w", err)
		}

		if ip.Valid {
			value := ip.String
			miner.IP = &value
		}
		if apiKey.Valid {
			value := apiKey.String
			miner.APIKey = &value
		}
		miner.Managed = managedInt != 0

		if modelID.Valid {
			model, err := s.getModelByIDTx(ctx, tx, modelID.Int64)
			if err != nil {
				return nil, err
			}
			miner.Model = &model
		}

		if settingsID.Valid {
			settings, err := s.getSettingsByIDTx(ctx, tx, settingsID.Int64)
			if err != nil {
				return nil, err
			}
			miner.Settings = &settings
		}

		if latestStatusID.Valid {
			id := latestStatusID.Int64
			miner.LatestStatusID = &id
			status, err := s.getStatusByIDTx(ctx, tx, id)
			if err != nil {
				return nil, err
			}
			miner.LatestStatus = &status
		}

		miners = append(miners, miner)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate miners: %w", err)
	}

	return miners, nil
}
