package session

import (
	"fmt"
	"time"
)

func toISOFormat(value time.Time) string {
	return value.UTC().Format("2006-01-02T15:04:05+00:00")
}

func parseTime(raw string, field string) (time.Time, error) {
	if raw == "" {
		return time.Time{}, fmt.Errorf("session %s must be a non-empty string", field)
	}
	value, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("session %s is not valid ISO datetime: %s", field, raw)
	}
	return value, nil
}

func expiresAt(record Record) (time.Time, error) {
	if record.ExpiresAt != "" {
		return parseTime(record.ExpiresAt, "expires_at")
	}
	if record.TTLSeconds <= 0 {
		return time.Time{}, fmt.Errorf("session ttl_seconds must be a positive integer")
	}
	lastActiveAt, err := parseTime(record.LastActiveAt, "last_active_at")
	if err != nil {
		return time.Time{}, err
	}
	return lastActiveAt.Add(time.Duration(record.TTLSeconds) * time.Second), nil
}

func withStatus(record Record, status string, reason string) Record {
	updated := record
	now := toISOFormat(time.Now().UTC())
	updated.Status = status
	updated.LastError = &reason
	updated.UpdatedAt = &now
	if status == ExpiredStatus {
		updated.ExpiredAt = &now
	}
	if status == InvalidStatus {
		updated.InvalidatedAt = &now
	}
	return updated
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
