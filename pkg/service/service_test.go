package service

import (
	"testing"

	"mira/pkg/contract"
	"mira/pkg/runner"
	"mira/pkg/session"
	"mira/pkg/transport"
)

func request(sessionID *string, role string) contract.Request {
	return contract.Request{
		Version:       "v1",
		Role:          role,
		ContentFormat: "structured",
		Session:       contract.Session{Mode: sessionMode(sessionID), SessionID: sessionID, ContextHint: contract.SessionContextHint{WorkspaceRoot: testStringPtr("/tmp"), TaskDescription: testStringPtr("test"), GitBranch: testStringPtr("main")}},
		FileManifest:  contract.FileManifest{Mode: "none", Paths: []string{}, MaxTotalBytes: 512 * 1024, ReadOnly: true},
		Task:          "demo",
		Constraints:   []string{},
		Context:       contract.Context{Diff: "", Files: []contract.ContextFile{}, Docs: []contract.ContextDoc{}},
		MaxTokens:     512,
		TimeoutSec:    30,
	}
}

func TestServiceRejectsExpiredSession(t *testing.T) {
	store, err := session.New(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	record, err := session.BuildRecord(request(testStringPtr("sticky-expired"), "planner"), "remote-session-1", nil)
	if err != nil {
		t.Fatalf("build record: %v", err)
	}
	record.ExpiresAt = "2000-01-01T00:00:00+00:00"
	if err := store.Save("sticky-expired", record); err != nil {
		t.Fatalf("save: %v", err)
	}
	response, exitCode, err := Service{Client: transport.FakeTransport{}, Store: store}.Run(request(testStringPtr("sticky-expired"), "planner"))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if exitCode != runner.ExitInvalidRequest {
		t.Fatalf("exit code = %d", exitCode)
	}
	if response.Errors[0].Code != "session_expired" {
		t.Fatalf("code = %s", response.Errors[0].Code)
	}
}

func TestServiceMarksInvalidSession(t *testing.T) {
	store, err := session.New(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	record, err := session.BuildRecord(request(testStringPtr("sticky-active"), "planner"), "remote-session-1", nil)
	if err != nil {
		t.Fatalf("build record: %v", err)
	}
	if err := store.Save("sticky-active", record); err != nil {
		t.Fatalf("save: %v", err)
	}
	client := &fakeSessionTransport{
		FakeTransport: transport.FakeTransport{ExecuteErr: transport.APIError{Message: "conversation not found"}},
		sessionID:     "remote-session-1",
		modelName:     testStringPtr("test-model"),
	}
	response, exitCode, err := Service{Client: client, Store: store}.Run(request(testStringPtr("sticky-active"), "planner"))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if exitCode != runner.ExitInvalidRequest {
		t.Fatalf("exit code = %d", exitCode)
	}
	if response.Errors[0].Code != "invalid_session" {
		t.Fatalf("code = %s", response.Errors[0].Code)
	}
}

type fakeSessionTransport struct {
	transport.FakeTransport
	sessionID string
	modelName *string
}

func (transport *fakeSessionTransport) SetRemoteSessionID(sessionID string) {
	transport.sessionID = sessionID
}
func (transport *fakeSessionTransport) RemoteSessionID() string { return transport.sessionID }
func (transport *fakeSessionTransport) ModelName() *string      { return transport.modelName }
func (transport *fakeSessionTransport) HasAuth() bool           { return true }

func sessionMode(sessionID *string) string {
	if sessionID == nil {
		return "ephemeral"
	}
	return "sticky"
}

func testStringPtr(value string) *string {
	return &value
}
