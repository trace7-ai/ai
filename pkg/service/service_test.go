package service

import (
	"testing"

	"mira/pkg/contract"
	"mira/pkg/runner"
	"mira/pkg/session"
	"mira/pkg/transport"
)

func request(sessionID *string) contract.Request {
	return contract.Request{
		Version:       "v1",
		Role:          "assistant",
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

func TestServiceRevivesExpiredSession(t *testing.T) {
	store, err := session.New(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	record, err := session.BuildRecord(request(testStringPtr("sticky-expired")), "remote-session-1", nil)
	if err != nil {
		t.Fatalf("build record: %v", err)
	}
	record.Status = "expired"
	record.LastError = testStringPtr("legacy ttl expiry")
	if err := store.Save("sticky-expired", record); err != nil {
		t.Fatalf("save: %v", err)
	}
	response, exitCode, err := Service{Client: transport.FakeTransport{
		Events: []transport.Event{{Type: "content", Text: "{}"}, {Type: "done"}},
	}, Store: store}.Run(request(testStringPtr("sticky-expired")))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if exitCode != runner.ExitOK {
		t.Fatalf("exit code = %d", exitCode)
	}
	if response.Status != "ok" {
		t.Fatalf("status = %s", response.Status)
	}
	snapshot, err := store.Inspect("sticky-expired")
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if snapshot.Status != session.ActiveStatus {
		t.Fatalf("status = %s", snapshot.Status)
	}
	if snapshot.Record == nil || snapshot.Record.Status != session.ActiveStatus || snapshot.Record.LastError != nil {
		t.Fatalf("record = %+v", snapshot.Record)
	}
}

func TestServiceMarksInvalidSession(t *testing.T) {
	store, err := session.New(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	record, err := session.BuildRecord(request(testStringPtr("sticky-active")), "remote-session-1", nil)
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
	response, exitCode, err := Service{Client: client, Store: store}.Run(request(testStringPtr("sticky-active")))
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

func TestServiceAllowsStickySessionAcrossRoles(t *testing.T) {
	store, err := session.New(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	record, err := session.BuildRecord(request(testStringPtr("sticky-shared")), "remote-session-1", nil)
	if err != nil {
		t.Fatalf("build record: %v", err)
	}
	if err := store.Save("sticky-shared", record); err != nil {
		t.Fatalf("save: %v", err)
	}
	client := &fakeSessionTransport{
		FakeTransport: transport.FakeTransport{
			Events: []transport.Event{
				{Type: "content", Text: "assistant ok"},
				{Type: "done"},
			},
		},
		sessionID: "remote-session-1",
		modelName: testStringPtr("test-model"),
	}
	stickyRequest := request(testStringPtr("sticky-shared"))
	stickyRequest.ContentFormat = "text"
	response, exitCode, err := Service{Client: client, Store: store}.Run(stickyRequest)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if exitCode != runner.ExitOK {
		t.Fatalf("exit code = %d", exitCode)
	}
	if response.Status != "ok" {
		t.Fatalf("status = %s", response.Status)
	}
	if response.Session == nil || response.Session.TurnIndex != 2 {
		t.Fatalf("session = %+v", response.Session)
	}
	if !response.Session.Reused {
		t.Fatalf("expected reused session payload")
	}
	snapshot, err := store.Inspect("sticky-shared")
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if snapshot.Record == nil || snapshot.Record.Role != "assistant" {
		t.Fatalf("record = %+v", snapshot.Record)
	}
	if snapshot.Record.RemoteSessionID != "remote-session-1" {
		t.Fatalf("remote session = %s", snapshot.Record.RemoteSessionID)
	}
	entries, err := store.ReadJournal("sticky-shared", 10)
	if err != nil {
		t.Fatalf("read journal: %v", err)
	}
	if len(entries) != 1 || entries[0].Summary != "assistant ok" {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestServiceReportsNewStickySessionAsNotReused(t *testing.T) {
	store, err := session.New(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	client := &fakeSessionTransport{
		FakeTransport: transport.FakeTransport{
			Events: []transport.Event{
				{Type: "content", Text: "assistant ok"},
				{Type: "done"},
			},
		},
		sessionID: "remote-session-1",
		modelName: testStringPtr("test-model"),
	}
	request := request(testStringPtr("fresh-sticky"))
	request.ContentFormat = "text"
	response, exitCode, err := Service{Client: client, Store: store}.Run(request)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if exitCode != runner.ExitOK {
		t.Fatalf("exit code = %d", exitCode)
	}
	if response.Session == nil {
		t.Fatalf("expected session payload")
	}
	if response.Session.Reused {
		t.Fatalf("expected fresh sticky session, got reused")
	}
}

func TestServiceEphemeralReturnsStructuredSessionPayload(t *testing.T) {
	store, err := session.New(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	response, exitCode, err := Service{
		Client: transport.FakeTransport{
			Events: []transport.Event{
				{Type: "content", Text: "assistant ok"},
				{Type: "done"},
			},
		},
		Store: store,
	}.Run(func() contract.Request {
		req := request(nil)
		req.ContentFormat = "text"
		return req
	}())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if exitCode != runner.ExitOK {
		t.Fatalf("exit code = %d", exitCode)
	}
	if response.Session == nil {
		t.Fatalf("expected session payload")
	}
	if response.Session.Status != "ephemeral" {
		t.Fatalf("status = %s, want ephemeral", response.Session.Status)
	}
	if response.Session.SessionID != "" {
		t.Fatalf("session_id = %q, want empty", response.Session.SessionID)
	}
	if response.Session.Reused {
		t.Fatalf("expected ephemeral session to never be reused")
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
