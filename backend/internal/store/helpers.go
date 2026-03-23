package store

import (
	"database/sql"
	"time"
)

const timeLayout = time.RFC3339

func toNullableString(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

func toNullableTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC().Format(timeLayout)
}

func toSQLiteBool(value bool) int {
	if value {
		return 1
	}
	return 0
}

func toNullableInt(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}

func toNullableFloat(value *float64) any {
	if value == nil {
		return nil
	}
	return *value
}

func parseNullableString(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	result := value.String
	return &result
}

func parseNullableTime(value sql.NullString) (*time.Time, error) {
	if !value.Valid || value.String == "" {
		return nil, nil
	}

	parsed, err := time.Parse(timeLayout, value.String)
	if err != nil {
		return nil, err
	}

	return &parsed, nil
}

func parseNullableInt(value sql.NullInt64) *int {
	if !value.Valid {
		return nil
	}
	result := int(value.Int64)
	return &result
}

func parseNullableFloat(value sql.NullFloat64) *float64 {
	if !value.Valid {
		return nil
	}
	result := value.Float64
	return &result
}
