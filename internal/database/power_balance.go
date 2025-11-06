package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// RecordPowerBalanceEvent logs a power balancing preset change event.
func (s *Store) RecordPowerBalanceEvent(ctx context.Context, input PowerBalanceEventInput) (PowerBalanceEvent, error) {
	minerID := strings.TrimSpace(input.MinerID)
	if minerID == "" {
		return PowerBalanceEvent{}, fmt.Errorf("miner id is required")
	}

	recordedAt := input.RecordedAt
	if recordedAt.IsZero() {
		recordedAt = time.Now().UTC()
	}

	successInt := 0
	if input.Success {
		successInt = 1
	}

	res, err := s.db.ExecContext(ctx, `
		INSERT INTO power_balance_events (
			miner_id, old_preset, new_preset, old_power, new_power, reason,
			total_consumption_before, total_consumption_after, available_power, target_power,
			success, error_message, recorded_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, minerID,
		nullableString(input.OldPreset),
		nullableString(input.NewPreset),
		nullableFloat64(input.OldPower),
		nullableFloat64(input.NewPower),
		input.Reason,
		nullableFloat64(input.TotalConsumptionBefore),
		nullableFloat64(input.TotalConsumptionAfter),
		nullableFloat64(input.AvailablePower),
		nullableFloat64(input.TargetPower),
		successInt,
		nullableString(input.ErrorMessage),
		recordedAt)
	if err != nil {
		return PowerBalanceEvent{}, fmt.Errorf("insert power balance event for miner %s: %w", minerID, err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return PowerBalanceEvent{}, fmt.Errorf("read power balance event id: %w", err)
	}

	return s.GetPowerBalanceEventByID(ctx, id)
}

// GetPowerBalanceEventByID retrieves a single power balance event by ID.
func (s *Store) GetPowerBalanceEventByID(ctx context.Context, id int64) (PowerBalanceEvent, error) {
	var (
		event        PowerBalanceEvent
		oldPreset    sql.NullString
		newPreset    sql.NullString
		oldPower     sql.NullFloat64
		newPower     sql.NullFloat64
		consumBefore sql.NullFloat64
		consumAfter  sql.NullFloat64
		availPower   sql.NullFloat64
		targetPower  sql.NullFloat64
		successInt   int
		errorMsg     sql.NullString
	)

	err := s.db.QueryRowContext(ctx, `
		SELECT id, miner_id, old_preset, new_preset, old_power, new_power, reason,
			total_consumption_before, total_consumption_after, available_power, target_power,
			success, error_message, recorded_at
		FROM power_balance_events
		WHERE id = ?
	`, id).Scan(&event.ID, &event.MinerID, &oldPreset, &newPreset, &oldPower, &newPower, &event.Reason,
		&consumBefore, &consumAfter, &availPower, &targetPower, &successInt, &errorMsg, &event.RecordedAt)
	if err != nil {
		return PowerBalanceEvent{}, fmt.Errorf("query power balance event %d: %w", id, err)
	}

	event.OldPreset = stringPtrFromNull(oldPreset)
	event.NewPreset = stringPtrFromNull(newPreset)
	event.OldPower = floatPtrFromNull(oldPower)
	event.NewPower = floatPtrFromNull(newPower)
	event.TotalConsumptionBefore = floatPtrFromNull(consumBefore)
	event.TotalConsumptionAfter = floatPtrFromNull(consumAfter)
	event.AvailablePower = floatPtrFromNull(availPower)
	event.TargetPower = floatPtrFromNull(targetPower)
	event.Success = successInt == 1
	event.ErrorMessage = stringPtrFromNull(errorMsg)

	return event, nil
}

// ListPowerBalanceEvents returns recent power balance events, optionally filtered by miner.
func (s *Store) ListPowerBalanceEvents(ctx context.Context, minerID *string, limit int) ([]PowerBalanceEvent, error) {
	if limit <= 0 {
		limit = 100
	}

	query := `
		SELECT id, miner_id, old_preset, new_preset, old_power, new_power, reason,
			total_consumption_before, total_consumption_after, available_power, target_power,
			success, error_message, recorded_at
		FROM power_balance_events
	`
	args := []any{}

	if minerID != nil && *minerID != "" {
		query += " WHERE miner_id = ?"
		args = append(args, *minerID)
	}

	query += " ORDER BY recorded_at DESC, id DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query power balance events: %w", err)
	}
	defer rows.Close()

	var events []PowerBalanceEvent
	for rows.Next() {
		var (
			event        PowerBalanceEvent
			oldPreset    sql.NullString
			newPreset    sql.NullString
			oldPower     sql.NullFloat64
			newPower     sql.NullFloat64
			consumBefore sql.NullFloat64
			consumAfter  sql.NullFloat64
			availPower   sql.NullFloat64
			targetPower  sql.NullFloat64
			successInt   int
			errorMsg     sql.NullString
		)

		if err := rows.Scan(&event.ID, &event.MinerID, &oldPreset, &newPreset, &oldPower, &newPower, &event.Reason,
			&consumBefore, &consumAfter, &availPower, &targetPower, &successInt, &errorMsg, &event.RecordedAt); err != nil {
			return nil, fmt.Errorf("scan power balance event: %w", err)
		}

		event.OldPreset = stringPtrFromNull(oldPreset)
		event.NewPreset = stringPtrFromNull(newPreset)
		event.OldPower = floatPtrFromNull(oldPower)
		event.NewPower = floatPtrFromNull(newPower)
		event.TotalConsumptionBefore = floatPtrFromNull(consumBefore)
		event.TotalConsumptionAfter = floatPtrFromNull(consumAfter)
		event.AvailablePower = floatPtrFromNull(availPower)
		event.TargetPower = floatPtrFromNull(targetPower)
		event.Success = successInt == 1
		event.ErrorMessage = stringPtrFromNull(errorMsg)

		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate power balance events: %w", err)
	}

	return events, nil
}
