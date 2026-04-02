package session

import (
	"fmt"
	"time"
)

func (store *Store) Inspect(sessionID string) (Snapshot, error) {
	if err := requireSessionID(sessionID); err != nil {
		return Snapshot{}, err
	}
	record, err := store.Load(sessionID)
	if err != nil {
		return Snapshot{}, err
	}
	if record == nil {
		return Snapshot{Status: MissingStatus}, nil
	}
	if record.Status == ActiveStatus {
		return store.inspectActive(sessionID, *record)
	}
	if record.Status == ExpiredStatus || record.Status == InvalidStatus {
		return Snapshot{Status: record.Status, Record: record, Reason: record.LastError}, nil
	}
	reason := fmt.Sprintf("unsupported session status: %s", record.Status)
	return store.markInvalidSnapshot(sessionID, *record, reason)
}

func (store *Store) inspectActive(sessionID string, record Record) (Snapshot, error) {
	if record.RemoteSessionID == "" {
		return store.markInvalidSnapshot(sessionID, record, "active session is missing remote_session_id")
	}
	if record.TurnCount < 0 {
		return store.markInvalidSnapshot(sessionID, record, "active session has invalid turn_count")
	}
	expiresAt, err := expiresAt(record)
	if err != nil {
		return Snapshot{}, err
	}
	if expiresAt.Before(time.Now().UTC()) {
		expired := withStatus(record, ExpiredStatus, "session ttl elapsed")
		if err := store.Save(sessionID, expired); err != nil {
			return Snapshot{}, err
		}
		return Snapshot{Status: ExpiredStatus, Record: &expired, Reason: expired.LastError}, nil
	}
	active := record
	iso := toISOFormat(expiresAt)
	active.ExpiresAt = iso
	return Snapshot{Status: ActiveStatus, Record: &active}, nil
}

func (store *Store) markInvalidSnapshot(sessionID string, record Record, reason string) (Snapshot, error) {
	invalid, err := store.MarkInvalid(sessionID, record, reason)
	if err != nil {
		return Snapshot{}, err
	}
	return Snapshot{Status: InvalidStatus, Record: &invalid, Reason: invalid.LastError}, nil
}
