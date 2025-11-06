package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// RecordMinerStatus persists a miner status snapshot and marks it as the latest.
func (s *Store) RecordMinerStatus(ctx context.Context, minerID string, input MinerStatusInput) (Status, error) {
	minerID = strings.TrimSpace(minerID)
	if minerID == "" {
		return Status{}, fmt.Errorf("miner id is required")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Status{}, fmt.Errorf("begin record status tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	recordedAt := input.RecordedAt
	if recordedAt.IsZero() {
		recordedAt = time.Now().UTC()
	}

	res, err := tx.ExecContext(ctx, `
		INSERT INTO statuses (miner_id, uptime, state, preset, hashrate, power_usage, power_consumption, recorded_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, minerID,
		nullableInt64(input.Uptime),
		nullableTrimmedString(input.State),
		nullableTrimmedString(input.Preset),
		nullableFloat64(input.Hashrate),
		nullableFloat64(input.PowerUsage),
		nullableFloat64(input.PowerConsumption),
		recordedAt)
	if err != nil {
		return Status{}, fmt.Errorf("insert status for miner %s: %w", minerID, err)
	}

	statusID, err := res.LastInsertId()
	if err != nil {
		return Status{}, fmt.Errorf("read status id: %w", err)
	}

	for _, fan := range input.Fans {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO status_fans (status_id, fan_identifier, rpm, status)
			VALUES (?, ?, ?, ?)
		`, statusID, nullableTrimmedString(fan.FanIdentifier), nullableInt(fan.RPM), nullableTrimmedString(fan.Status)); err != nil {
			return Status{}, fmt.Errorf("insert fan status: %w", err)
		}
	}

	for _, chain := range input.Chains {
		chainRes, err := tx.ExecContext(ctx, `
			INSERT INTO chain_snapshots (
				miner_id,
				status_id,
				chain_identifier,
				state,
				hashrate,
				pcb_temp_min,
				pcb_temp_max,
				chip_temp_min,
				chip_temp_max,
				recorded_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, minerID,
			statusID,
			nullableTrimmedString(chain.ChainIdentifier),
			nullableTrimmedString(chain.State),
			nullableFloat64(chain.Hashrate),
			nullableFloat64(chain.PCBTempMin),
			nullableFloat64(chain.PCBTempMax),
			nullableFloat64(chain.ChipTempMin),
			nullableFloat64(chain.ChipTempMax),
			recordedAt)
		if err != nil {
			return Status{}, fmt.Errorf("insert chain snapshot: %w", err)
		}

		chainID, err := chainRes.LastInsertId()
		if err != nil {
			return Status{}, fmt.Errorf("read chain snapshot id: %w", err)
		}

		for _, chip := range chain.Chips {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO chain_chips (chain_snapshot_id, chip_identifier, hashrate, temperature)
				VALUES (?, ?, ?, ?)
			`, chainID, nullableTrimmedString(chip.ChipIdentifier), nullableFloat64(chip.Hashrate), nullableFloat64(chip.Temperature)); err != nil {
				return Status{}, fmt.Errorf("insert chip snapshot: %w", err)
			}
		}
	}

	result, err := tx.ExecContext(ctx, `
		UPDATE miners
		SET latest_status_id = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, statusID, minerID)
	if err != nil {
		return Status{}, fmt.Errorf("set latest status for miner %s: %w", minerID, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return Status{}, fmt.Errorf("status attach rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return Status{}, fmt.Errorf("miner %s not found", minerID)
	}

	if err := tx.Commit(); err != nil {
		return Status{}, fmt.Errorf("commit record status tx: %w", err)
	}

	return s.GetStatusByID(ctx, statusID)
}

// GetStatusByID retrieves a status snapshot with relations.
func (s *Store) GetStatusByID(ctx context.Context, statusID int64) (Status, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return Status{}, fmt.Errorf("begin get status tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	status, err := s.getStatusByIDTx(ctx, tx, statusID)
	if err != nil {
		return Status{}, err
	}

	if err := tx.Commit(); err != nil {
		return Status{}, fmt.Errorf("commit get status tx: %w", err)
	}

	return status, nil
}

// ListMinerStatuses returns the latest N status snapshots for a miner ordered by newest first.
func (s *Store) ListMinerStatuses(ctx context.Context, minerID string, limit int) ([]Status, error) {
	minerID = strings.TrimSpace(minerID)
	if minerID == "" {
		return nil, fmt.Errorf("miner id is required")
	}
	if limit <= 0 {
		limit = 20
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id
		FROM statuses
		WHERE miner_id = ?
		ORDER BY recorded_at DESC, id DESC
		LIMIT ?
	`, minerID, limit)
	if err != nil {
		return nil, fmt.Errorf("query miner statuses: %w", err)
	}
	defer rows.Close()

	var (
		ids      []int64
		statuses []Status
	)

	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan status id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate status ids: %w", err)
	}

	for _, id := range ids {
		status, err := s.GetStatusByID(ctx, id)
		if err != nil {
			return nil, err
		}
		statuses = append(statuses, status)
	}

	return statuses, nil
}

func (s *Store) getStatusByIDTx(ctx context.Context, tx *sql.Tx, statusID int64) (Status, error) {
	var (
		status           Status
		uptime           sql.NullInt64
		state            sql.NullString
		preset           sql.NullString
		hashrate         sql.NullFloat64
		powerUsage       sql.NullFloat64
		powerConsumption sql.NullFloat64
	)

	err := tx.QueryRowContext(ctx, `
		SELECT id, miner_id, uptime, state, preset, hashrate, power_usage, power_consumption, recorded_at
		FROM statuses
		WHERE id = ?
	`, statusID).Scan(&status.ID, &status.MinerID, &uptime, &state, &preset, &hashrate, &powerUsage, &powerConsumption, &status.RecordedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Status{}, fmt.Errorf("status %d not found", statusID)
		}
		return Status{}, fmt.Errorf("query status %d: %w", statusID, err)
	}

	status.Uptime = int64PtrFromNull(uptime)
	status.State = stringPtrFromNull(state)
	status.Hashrate = floatPtrFromNull(hashrate)
	status.Preset = stringPtrFromNull(preset)
	status.PowerUsage = floatPtrFromNull(powerUsage)
	status.PowerConsumption = floatPtrFromNull(powerConsumption)

	fans, err := loadStatusFans(ctx, tx, status.ID)
	if err != nil {
		return Status{}, err
	}
	status.Fans = fans

	chains, err := loadChainSnapshots(ctx, tx, status.ID)
	if err != nil {
		return Status{}, err
	}
	status.Chains = chains

	return status, nil
}

func loadStatusFans(ctx context.Context, tx *sql.Tx, statusID int64) ([]FanStatus, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, fan_identifier, rpm, status
		FROM status_fans
		WHERE status_id = ?
		ORDER BY id
	`, statusID)
	if err != nil {
		return nil, fmt.Errorf("query status fans: %w", err)
	}
	defer rows.Close()

	var fans []FanStatus
	for rows.Next() {
		var (
			fan        FanStatus
			identifier sql.NullString
			rpm        sql.NullInt64
			fanState   sql.NullString
		)

		if err := rows.Scan(&fan.ID, &identifier, &rpm, &fanState); err != nil {
			return nil, fmt.Errorf("scan fan status: %w", err)
		}
		fan.StatusID = statusID
		fan.FanIdentifier = stringPtrFromNull(identifier)
		fan.RPM = intPtrFromNull(rpm)
		fan.Status = stringPtrFromNull(fanState)
		fans = append(fans, fan)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate fan statuses: %w", err)
	}

	return fans, nil
}

func loadChainSnapshots(ctx context.Context, tx *sql.Tx, statusID int64) ([]ChainSnapshot, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT
			id,
			miner_id,
			chain_identifier,
			state,
			hashrate,
			pcb_temp_min,
			pcb_temp_max,
			chip_temp_min,
			chip_temp_max,
			recorded_at
		FROM chain_snapshots
		WHERE status_id = ?
		ORDER BY recorded_at, id
	`, statusID)
	if err != nil {
		return nil, fmt.Errorf("query chain snapshots: %w", err)
	}
	defer rows.Close()

	var snapshots []ChainSnapshot
	for rows.Next() {
		var (
			snapshot        ChainSnapshot
			chainIdentifier sql.NullString
			state           sql.NullString
			hashrate        sql.NullFloat64
			pcbTempMin      sql.NullFloat64
			pcbTempMax      sql.NullFloat64
			chipTempMin     sql.NullFloat64
			chipTempMax     sql.NullFloat64
		)

		if err := rows.Scan(
			&snapshot.ID,
			&snapshot.MinerID,
			&chainIdentifier,
			&state,
			&hashrate,
			&pcbTempMin,
			&pcbTempMax,
			&chipTempMin,
			&chipTempMax,
			&snapshot.RecordedAt,
		); err != nil {
			return nil, fmt.Errorf("scan chain snapshot: %w", err)
		}
		snapshot.StatusID = &statusID
		snapshot.ChainIdentifier = stringPtrFromNull(chainIdentifier)
		snapshot.State = stringPtrFromNull(state)
		snapshot.Hashrate = floatPtrFromNull(hashrate)
		snapshot.PCBTempMin = floatPtrFromNull(pcbTempMin)
		snapshot.PCBTempMax = floatPtrFromNull(pcbTempMax)
		snapshot.ChipTempMin = floatPtrFromNull(chipTempMin)
		snapshot.ChipTempMax = floatPtrFromNull(chipTempMax)

		chips, err := loadChipSnapshots(ctx, tx, snapshot.ID)
		if err != nil {
			return nil, err
		}
		snapshot.Chips = chips
		snapshots = append(snapshots, snapshot)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate chain snapshots: %w", err)
	}

	return snapshots, nil
}

func loadChipSnapshots(ctx context.Context, tx *sql.Tx, chainSnapshotID int64) ([]ChipSnapshot, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, chip_identifier, hashrate, temperature
		FROM chain_chips
		WHERE chain_snapshot_id = ?
		ORDER BY id
	`, chainSnapshotID)
	if err != nil {
		return nil, fmt.Errorf("query chip snapshots: %w", err)
	}
	defer rows.Close()

	var chips []ChipSnapshot
	for rows.Next() {
		var (
			chip           ChipSnapshot
			chipIdentifier sql.NullString
			hashrate       sql.NullFloat64
			temp           sql.NullFloat64
		)

		if err := rows.Scan(&chip.ID, &chipIdentifier, &hashrate, &temp); err != nil {
			return nil, fmt.Errorf("scan chip snapshot: %w", err)
		}
		chip.ChainSnapshotID = chainSnapshotID
		chip.ChipIdentifier = stringPtrFromNull(chipIdentifier)
		chip.Hashrate = floatPtrFromNull(hashrate)
		chip.Temperature = floatPtrFromNull(temp)
		chips = append(chips, chip)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate chip snapshots: %w", err)
	}

	return chips, nil
}
