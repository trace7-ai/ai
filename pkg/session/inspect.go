package session

import "fmt"

const legacyExpiredStatus = "expired"

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
	if record.Status == ActiveStatus || record.Status == legacyExpiredStatus {
		return store.inspectActive(sessionID, *record)
	}
	if record.Status == InvalidStatus {
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
	active, changed := normalizePersistentRecord(record)
	if changed {
		if err := store.Save(sessionID, active); err != nil {
			return Snapshot{}, err
		}
	}
	return Snapshot{Status: ActiveStatus, Record: &active}, nil
}

func (store *Store) markInvalidSnapshot(sessionID string, record Record, reason string) (Snapshot, error) {
	invalid, err := store.MarkInvalid(sessionID, record, reason)
	if err != nil {
		return Snapshot{}, err
	}
	return Snapshot{Status: InvalidStatus, Record: &invalid, Reason: invalid.LastError}, nil
}

func normalizePersistentRecord(record Record) (Record, bool) {
	normalized := record
	changed := false
	if normalized.Status != ActiveStatus {
		normalized.Status = ActiveStatus
		changed = true
	}
	if normalized.LastError != nil {
		normalized.LastError = nil
		changed = true
	}
	return normalized, changed
}
