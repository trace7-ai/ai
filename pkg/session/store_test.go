package session

import (
	"testing"

	"mira/pkg/contract"
)

func request(sessionID string) contract.Request {
	return contract.Request{
		Version:       "v1",
		Role:          "assistant",
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

func TestInspectRevivesExpiredSessionAsPermanent(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	record, err := BuildRecord(request("sticky-expired"), "remote-session-1", nil)
	if err != nil {
		t.Fatalf("build record: %v", err)
	}
	record.Status = "expired"
	record.LastError = stringPtr("legacy ttl expiry")
	if err := store.Save("sticky-expired", record); err != nil {
		t.Fatalf("save: %v", err)
	}
	snapshot, err := store.Inspect("sticky-expired")
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if snapshot.Status != ActiveStatus {
		t.Fatalf("status = %s, want %s", snapshot.Status, ActiveStatus)
	}
	if snapshot.Record == nil || snapshot.Record.Status != ActiveStatus || snapshot.Record.LastError != nil {
		t.Fatalf("record = %+v", snapshot.Record)
	}
}

func TestCompatibilityOnlyChecksWorkspace(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	record, err := BuildRecord(request("sticky-active"), "remote-session-1", nil)
	if err != nil {
		t.Fatalf("build record: %v", err)
	}
	message := store.CompatibilityError(&record, request("sticky-active"))
	if message != nil {
		t.Fatalf("message = %v", message)
	}
}

func TestCompatibilityErrorOnWorkspaceMismatch(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	record, err := BuildRecord(request("sticky-active"), "remote-session-1", nil)
	if err != nil {
		t.Fatalf("build record: %v", err)
	}
	mismatched := request("sticky-active")
	mismatched.Session.ContextHint.WorkspaceRoot = stringPtr("/var/tmp")
	message := store.CompatibilityError(&record, mismatched)
	if message == nil || *message != "session workspace mismatch: expected /tmp got /var/tmp" {
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

func TestLockTaskPinCreatesLockFile(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	unlock, err := store.LockTaskPin("/tmp/work")
	if err != nil {
		t.Fatalf("lock task pin: %v", err)
	}
	if err := unlock(); err != nil {
		t.Fatalf("unlock: %v", err)
	}
}

func TestAppendAndReadJournal(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	entry := JournalEntry{
		Turn:      1,
		Timestamp: "2026-04-03T00:00:00Z",
		SessionID: "demo",
		Task:      "review",
		Summary:   "found issue",
		Status:    "ok",
	}
	if err := store.AppendJournalEntry("demo", entry); err != nil {
		t.Fatalf("append journal: %v", err)
	}
	entries, err := store.ReadJournal("demo", 10)
	if err != nil {
		t.Fatalf("read journal: %v", err)
	}
	if len(entries) != 1 || entries[0].Summary != "found issue" {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestSaveAndLoadTaskPin(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	task := "review current task chain"
	if err := store.SaveTaskPin("/tmp/work", "task-chain-1", &task); err != nil {
		t.Fatalf("save pin: %v", err)
	}
	pin, err := store.LoadTaskPin("/tmp/work")
	if err != nil {
		t.Fatalf("load pin: %v", err)
	}
	if pin == nil || pin.SessionID != "task-chain-1" {
		t.Fatalf("pin = %+v", pin)
	}
	if pin.TaskDescription == nil || *pin.TaskDescription != task {
		t.Fatalf("task description = %+v", pin)
	}
}

func stringPtr(value string) *string {
	return &value
}
