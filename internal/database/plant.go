package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// RecordPlantReading persists a plant energy generation/consumption snapshot.
func (s *Store) RecordPlantReading(ctx context.Context, input PlantReadingInput) (PlantReading, error) {
	recordedAt := input.RecordedAt
	if recordedAt.IsZero() {
		recordedAt = time.Now().UTC()
	}

	res, err := s.db.ExecContext(ctx, `
		INSERT INTO plant_readings (plant_id, total_generation, total_container_consumption, available_power, raw_data, recorded_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, input.PlantID, input.TotalGeneration, input.TotalContainerConsumption, input.AvailablePower,
		nullableString(input.RawData), recordedAt)
	if err != nil {
		return PlantReading{}, fmt.Errorf("insert plant reading: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return PlantReading{}, fmt.Errorf("read plant reading id: %w", err)
	}

	return s.GetPlantReadingByID(ctx, id)
}

// GetPlantReadingByID retrieves a single plant reading by its ID.
func (s *Store) GetPlantReadingByID(ctx context.Context, id int64) (PlantReading, error) {
	var (
		reading PlantReading
		rawData sql.NullString
	)

	err := s.db.QueryRowContext(ctx, `
		SELECT id, plant_id, total_generation, total_container_consumption, available_power, raw_data, recorded_at
		FROM plant_readings
		WHERE id = ?
	`, id).Scan(&reading.ID, &reading.PlantID, &reading.TotalGeneration, &reading.TotalContainerConsumption,
		&reading.AvailablePower, &rawData, &reading.RecordedAt)
	if err != nil {
		return PlantReading{}, fmt.Errorf("query plant reading %d: %w", id, err)
	}

	reading.RawData = stringPtrFromNull(rawData)
	return reading, nil
}

// GetLatestPlantReading returns the most recent plant reading.
func (s *Store) GetLatestPlantReading(ctx context.Context) (*PlantReading, error) {
	var (
		reading PlantReading
		rawData sql.NullString
	)

	err := s.db.QueryRowContext(ctx, `
		SELECT id, plant_id, total_generation, total_container_consumption, available_power, raw_data, recorded_at
		FROM plant_readings
		ORDER BY recorded_at DESC, id DESC
		LIMIT 1
	`).Scan(&reading.ID, &reading.PlantID, &reading.TotalGeneration, &reading.TotalContainerConsumption,
		&reading.AvailablePower, &rawData, &reading.RecordedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("query latest plant reading: %w", err)
	}

	reading.RawData = stringPtrFromNull(rawData)
	return &reading, nil
}

// ListPlantReadings returns recent plant readings ordered by time descending.
func (s *Store) ListPlantReadings(ctx context.Context, limit int) ([]PlantReading, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, plant_id, total_generation, total_container_consumption, available_power, raw_data, recorded_at
		FROM plant_readings
		ORDER BY recorded_at DESC, id DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("query plant readings: %w", err)
	}
	defer rows.Close()

	var readings []PlantReading
	for rows.Next() {
		var (
			reading PlantReading
			rawData sql.NullString
		)

		if err := rows.Scan(&reading.ID, &reading.PlantID, &reading.TotalGeneration,
			&reading.TotalContainerConsumption, &reading.AvailablePower, &rawData, &reading.RecordedAt); err != nil {
			return nil, fmt.Errorf("scan plant reading: %w", err)
		}

		reading.RawData = stringPtrFromNull(rawData)
		readings = append(readings, reading)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate plant readings: %w", err)
	}

	return readings, nil
}

// GetAppSetting retrieves a setting value by key.
func (s *Store) GetAppSetting(ctx context.Context, key string) (string, error) {
	var value string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM app_settings WHERE key = ?`, key).Scan(&value)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("setting %q not found", key)
		}
		return "", fmt.Errorf("query setting %q: %w", key, err)
	}
	return value, nil
}

// SetAppSetting updates or inserts a setting value.
func (s *Store) SetAppSetting(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO app_settings (key, value, updated_at)
		VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP
	`, key, value)
	if err != nil {
		return fmt.Errorf("set setting %q: %w", key, err)
	}
	return nil
}
