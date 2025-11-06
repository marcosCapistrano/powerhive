package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// RecordChainTelemetry stores per-chain and per-chip telemetry without tying it
// to a status snapshot.
func (s *Store) RecordChainTelemetry(ctx context.Context, minerID string, recordedAt time.Time, chains []ChainSnapshotInput) error {
	minerID = strings.TrimSpace(minerID)
	if minerID == "" {
		return fmt.Errorf("miner id is required")
	}
	if len(chains) == 0 {
		return nil
	}
	if recordedAt.IsZero() {
		recordedAt = time.Now().UTC()
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin telemetry tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, chain := range chains {
		res, err := tx.ExecContext(ctx, `
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
            ) VALUES (?, NULL, ?, ?, ?, ?, ?, ?, ?, ?)
        `, minerID,
			nullableTrimmedString(chain.ChainIdentifier),
			nullableTrimmedString(chain.State),
			nullableFloat64(chain.Hashrate),
			nullableFloat64(chain.PCBTempMin),
			nullableFloat64(chain.PCBTempMax),
			nullableFloat64(chain.ChipTempMin),
			nullableFloat64(chain.ChipTempMax),
			recordedAt)
		if err != nil {
			return fmt.Errorf("insert chain telemetry: %w", err)
		}

		chainID, err := res.LastInsertId()
		if err != nil {
			return fmt.Errorf("chain telemetry id: %w", err)
		}

		for _, chip := range chain.Chips {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO chain_chips (chain_snapshot_id, chip_identifier, hashrate, temperature)
				VALUES (?, ?, ?, ?)
			`, chainID, nullableTrimmedString(chip.ChipIdentifier), nullableFloat64(chip.Hashrate), nullableFloat64(chip.Temperature)); err != nil {
				return fmt.Errorf("insert chip telemetry: %w", err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit telemetry tx: %w", err)
	}

	return nil
}

// ListChainTelemetry returns recent chain snapshots (including chip metrics) for a miner.
func (s *Store) ListChainTelemetry(ctx context.Context, minerID string, limit int) ([]ChainSnapshot, error) {
	minerID = strings.TrimSpace(minerID)
	if minerID == "" {
		return nil, fmt.Errorf("miner id is required")
	}
	if limit <= 0 {
		limit = 50
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("begin telemetry read tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx, `
        SELECT
            id,
            status_id,
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
        WHERE miner_id = ?
        ORDER BY recorded_at DESC, id DESC
        LIMIT ?
    `, minerID, limit)
	if err != nil {
		return nil, fmt.Errorf("query telemetry snapshots: %w", err)
	}
	defer rows.Close()

	var snapshots []ChainSnapshot
	for rows.Next() {
		var (
			snapshot        ChainSnapshot
			statusID        sql.NullInt64
			chainIdentifier sql.NullString
			state           sql.NullString
			hashrate        sql.NullFloat64
			pcbTempMin      sql.NullFloat64
			pcbTempMax      sql.NullFloat64
			chipTempMin     sql.NullFloat64
			chipTempMax     sql.NullFloat64
		)

		if err := rows.Scan(&snapshot.ID, &statusID, &snapshot.MinerID, &chainIdentifier, &state, &hashrate, &pcbTempMin, &pcbTempMax, &chipTempMin, &chipTempMax, &snapshot.RecordedAt); err != nil {
			return nil, fmt.Errorf("scan telemetry snapshot: %w", err)
		}

		if statusID.Valid {
			value := statusID.Int64
			snapshot.StatusID = &value
		}
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
		return nil, fmt.Errorf("iterate telemetry snapshots: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit telemetry read tx: %w", err)
	}

	return snapshots, nil
}
