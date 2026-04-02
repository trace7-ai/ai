package session

import (
	"testing"

	"mira/pkg/contract"
)

func request(sessionID string, role string) contract.Request {
	return contract.Request{
		Version:       "v1",
		Role:          role,
		RequestID:     nil,
		ContentFormat: "structured",
		Session: contract.Session{
			Mode:      "sticky",
			SessionID: stringPtr(sessionID),
			ContextHint: contract.SessionContextHint{
				WorkspaceRoot:   stringPtr("/tmp"),
				TaskDescription: stringPtr("test task"),
				GitBranch:       stringPtr("main"),
			},
		},
		FileManifest: contract.FileManifest{Mode: "none", Paths: []string{}, MaxTotalBytes: 512 * 1024, ReadOnly: true},
		Task:         "review the plan",
		Constraints:  []string{},
		Context:      contract.Context{Diff: "", Files: []contract.ContextFile{}, Docs: []contract.ContextDoc{}},
		MaxTokens:    512,
		TimeoutSec:   30,
	}
}

func TestInspectMarksExpiredSession(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	record, err := BuildRecord(request("sticky-expired", "planner"), "remote-session-1", nil)
	if err != nil {
		t.Fatalf("build record: %v", err)
	}
	record.ExpiresAt = "2000-01-01T00:00:00+00:00"
	if err := store.Save("sticky-expired", record); err != nil {
		t.Fatalf("save: %v", err)
	}
	snapshot, err := store.Inspect("sticky-expired")
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if snapshot.Status != ExpiredStatus {
		t.Fatalf("status = %s, want %s", snapshot.Status, ExpiredStatus)
	}
}

func TestCompatibilityErrorOnRoleMismatch(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	record, err := BuildRecord(request("sticky-active", "planner"), "remote-session-1", nil)
	if err != nil {
		t.Fatalf("build record: %v", err)
	}
	message := store.CompatibilityError(&record, request("sticky-active", "reviewer"))
	if message == nil || *message != "session role mismatch: expected planner got reviewer" {
		t.Fatalf("message = %v", message)
	}
}

func TestLockCreatesLockFile(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	unlock, err := store.Lock("demo")
	if err != nil {
		t.Fatalf("lock: %v", err)
	}
	if err := unlock(); err != nil {
		t.Fatalf("unlock: %v", err)
	}
}

func stringPtr(value string) *string {
	return &value
}
