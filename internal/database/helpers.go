package database

import (
	"database/sql"
	"strings"
)

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func nullableInt(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableInt64(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableFloat64(value *float64) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableString(value *string) any {
	if value == nil {
		return nil
	}
	return strings.TrimSpace(*value)
}

func nullableTrimmedString(value *string) any {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return trimmed
}

func stringPtrFromNull(ns sql.NullString) *string {
	if !ns.Valid {
		return nil
	}
	value := ns.String
	return &value
}

func intPtrFromNull(ns sql.NullInt64) *int {
	if !ns.Valid {
		return nil
	}
	value := int(ns.Int64)
	return &value
}

func int64PtrFromNull(ns sql.NullInt64) *int64 {
	if !ns.Valid {
		return nil
	}
	value := ns.Int64
	return &value
}

func floatPtrFromNull(ns sql.NullFloat64) *float64 {
	if !ns.Valid {
		return nil
	}
	value := ns.Float64
	return &value
}
