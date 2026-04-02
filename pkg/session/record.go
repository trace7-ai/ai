package session

import (
	"fmt"
	"time"

	"mira/pkg/contract"
)

func BuildRecord(request contract.Request, remoteSessionID string, existing *Record) (Record, error) {
	if remoteSessionID == "" {
		return Record{}, fmt.Errorf("remote_session_id must be a non-empty string")
	}
	now := time.Now().UTC()
	ttlSeconds := DefaultTTLSeconds
	if existing != nil {
		ttlSeconds = existing.TTLSeconds
	}
	turnCount := 1
	createdAt := toISOFormat(now)
	if existing != nil {
		turnCount = existing.TurnCount + 1
		createdAt = existing.CreatedAt
	}
	return Record{
		SessionID:       derefString(request.Session.SessionID),
		ParentSessionID: request.Session.ParentSessionID,
		CreatedAt:       createdAt,
		LastActiveAt:    toISOFormat(now),
		ExpiresAt:       toISOFormat(now.Add(time.Duration(ttlSeconds) * time.Second)),
		RemoteSessionID: remoteSessionID,
		TurnCount:       turnCount,
		Role:            request.Role,
		ContentFormat:   request.ContentFormat,
		WorkspaceRoot:   request.Session.ContextHint.WorkspaceRoot,
		TaskDescription: request.Session.ContextHint.TaskDescription,
		GitBranch:       request.Session.ContextHint.GitBranch,
		TTLSeconds:      ttlSeconds,
		Status:          ActiveStatus,
		LastError:       nil,
	}, nil
}

func (store *Store) CompatibilityError(record *Record, request contract.Request) *string {
	if record == nil {
		return nil
	}
	if record.Role != "" && record.Role != request.Role {
		message := fmt.Sprintf("session role mismatch: expected %s got %s", record.Role, request.Role)
		return &message
	}
	requestRoot := request.Session.ContextHint.WorkspaceRoot
	if record.WorkspaceRoot != nil && requestRoot != nil && *record.WorkspaceRoot != *requestRoot {
		message := fmt.Sprintf("session workspace mismatch: expected %s got %s", *record.WorkspaceRoot, *requestRoot)
		return &message
	}
	return nil
}

func (store *Store) MarkInvalid(sessionID string, record Record, reason string) (Record, error) {
	invalid := withStatus(record, InvalidStatus, reason)
	return invalid, store.Save(sessionID, invalid)
}
