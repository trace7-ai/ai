package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"mira/pkg/session"
)

func setTestMiraHome(t *testing.T) {
	t.Helper()
	t.Setenv("MIRA_HOME", t.TempDir())
}

func TestLegacyRoleAliasDefaultsToAssistantAuto(t *testing.T) {
	setTestMiraHome(t)
	request, err := LoadRequestFromCLI([]string{"--role", "reader", "--task", "read this doc"}, "/tmp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if request.ContentFormat != "auto" {
		t.Fatalf("content format = %s, want auto", request.ContentFormat)
	}
	if request.Role != "assistant" {
		t.Fatalf("role = %s, want assistant", request.Role)
	}
}

func TestDefaultRequestUsesAssistantWithoutRouting(t *testing.T) {
	setTestMiraHome(t)
	request, err := LoadRequestFromCLI([]string{"Plan the implementation steps"}, "/tmp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if request.Role != "assistant" {
		t.Fatalf("role = %s, want assistant", request.Role)
	}
	if request.Session.Mode != "sticky" {
		t.Fatalf("session mode = %s, want sticky", request.Session.Mode)
	}
	if request.Session.SessionID == nil || *request.Session.SessionID == "" {
		t.Fatalf("expected auto-generated session id")
	}
}

func TestContextDiffDoesNotSwitchRole(t *testing.T) {
	setTestMiraHome(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "context.json")
	body, _ := json.Marshal(map[string]any{
		"context": map[string]any{"diff": "diff --git a/app.py b/app.py", "files": []any{}, "docs": []any{}},
	})
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("write error: %v", err)
	}
	request, err := LoadRequestFromCLI([]string{"--context-file", path, "--task", "please check this patch"}, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if request.Role != "assistant" {
		t.Fatalf("role = %s, want assistant", request.Role)
	}
}

func TestRejectInvalidJSONContext(t *testing.T) {
	setTestMiraHome(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "context.json")
	if err := os.WriteFile(path, []byte("{bad json"), 0o644); err != nil {
		t.Fatalf("write error: %v", err)
	}
	_, err := LoadRequestFromCLI([]string{"--context-file", path, "summarize this"}, dir)
	if err == nil || err.Error() != "context file is not valid JSON: "+path {
		t.Fatalf("error = %v", err)
	}
}

func TestPromptModeReusesPinnedTaskSessionAcrossDifferentContexts(t *testing.T) {
	setTestMiraHome(t)
	dir := t.TempDir()
	firstPath := filepath.Join(dir, "context-a.md")
	secondPath := filepath.Join(dir, "context-b.md")
	if err := os.WriteFile(firstPath, []byte("main doc"), 0o644); err != nil {
		t.Fatalf("write error: %v", err)
	}
	if err := os.WriteFile(secondPath, []byte("qa checklist"), 0o644); err != nil {
		t.Fatalf("write error: %v", err)
	}
	first, err := LoadRequestFromCLI([]string{"--context-file", firstPath, "--task", "请审阅这个方案主文档"}, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	second, err := LoadRequestFromCLI([]string{"--context-file", secondPath, "--content-format", "text", "--task", "补充看看 QA 规范和验收口径"}, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if first.Session.SessionID == nil || second.Session.SessionID == nil {
		t.Fatalf("expected auto-generated session ids")
	}
	if *first.Session.SessionID != *second.Session.SessionID {
		t.Fatalf("session ids differ: %s vs %s", *first.Session.SessionID, *second.Session.SessionID)
	}
}

func TestExplicitSessionPinsCurrentTaskChain(t *testing.T) {
	setTestMiraHome(t)
	dir := t.TempDir()
	first, err := LoadRequestFromCLI([]string{"--session", "manual-task-chain", "--task", "开始新的大任务"}, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	second, err := LoadRequestFromCLI([]string{"--task", "继续这个大任务，换一份补充材料"}, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if first.Session.SessionID == nil || second.Session.SessionID == nil {
		t.Fatalf("expected session ids")
	}
	if *second.Session.SessionID != "manual-task-chain" {
		t.Fatalf("session id = %s, want manual-task-chain", *second.Session.SessionID)
	}
}

func TestEnvSessionIDPinsCurrentTaskChain(t *testing.T) {
	setTestMiraHome(t)
	t.Setenv("MIRA_SESSION_ID", "env-task-chain")
	dir := t.TempDir()
	first, err := LoadRequestFromCLI([]string{"--task", "开始一个由 env 注入的任务"}, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	second, err := LoadRequestFromCLI([]string{"--task", "继续这个任务"}, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if first.Session.SessionID == nil || second.Session.SessionID == nil {
		t.Fatalf("expected session ids")
	}
	if *first.Session.SessionID != "env-task-chain" || *second.Session.SessionID != "env-task-chain" {
		t.Fatalf("env session not reused: %v / %v", first.Session.SessionID, second.Session.SessionID)
	}
}

func TestNewSessionDetachesOldPinAndReplacesTaskChain(t *testing.T) {
	setTestMiraHome(t)
	dir := t.TempDir()
	first, err := LoadRequestFromCLI([]string{"--session", "task-a", "--task", "开始任务 A"}, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	second, err := LoadRequestFromCLI([]string{"--new-session", "--task", "开始完全无关的任务 B"}, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	third, err := LoadRequestFromCLI([]string{"--task", "继续任务 B"}, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if first.Session.SessionID == nil || second.Session.SessionID == nil || third.Session.SessionID == nil {
		t.Fatalf("expected session ids")
	}
	if *second.Session.SessionID == "task-a" {
		t.Fatalf("new session should detach from old pin")
	}
	if *third.Session.SessionID != *second.Session.SessionID {
		t.Fatalf("new task-chain pin not reused: %s vs %s", *second.Session.SessionID, *third.Session.SessionID)
	}
}

func TestInvalidPinnedSessionFallsBackToFreshAutoSession(t *testing.T) {
	setTestMiraHome(t)
	dir := t.TempDir()
	store, err := session.New("")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	invalidTask := "old broken task"
	if err := store.SaveTaskPin(dir, "invalid-session", &invalidTask); err != nil {
		t.Fatalf("save pin: %v", err)
	}
	record := session.Record{
		SessionID:       "invalid-session",
		CreatedAt:       "2026-04-03T00:00:00+00:00",
		LastActiveAt:    "2026-04-03T00:00:00+00:00",
		RemoteSessionID: "remote-invalid",
		TurnCount:       1,
		Role:            "assistant",
		ContentFormat:   "auto",
		WorkspaceRoot:   stringPtr(dir),
		Status:          session.InvalidStatus,
		LastError:       stringPtr("conversation not found"),
	}
	if err := store.Save("invalid-session", record); err != nil {
		t.Fatalf("save record: %v", err)
	}
	request, err := LoadRequestFromCLI([]string{"--task", "开始新的相关任务"}, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if request.Session.SessionID == nil {
		t.Fatalf("expected session id")
	}
	if *request.Session.SessionID == "invalid-session" {
		t.Fatalf("session id should not reuse invalid pin")
	}
}

func TestCorruptPinnedRecordFallsBackToFreshAutoSession(t *testing.T) {
	setTestMiraHome(t)
	dir := t.TempDir()
	store, err := session.New("")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	task := "old broken task"
	if err := store.SaveTaskPin(dir, "corrupt-session", &task); err != nil {
		t.Fatalf("save pin: %v", err)
	}
	recordPath := filepath.Join(os.Getenv("MIRA_HOME"), "sessions", "corrupt-session.json")
	if err := os.WriteFile(recordPath, []byte("{bad json"), 0o600); err != nil {
		t.Fatalf("write corrupt record: %v", err)
	}
	request, err := LoadRequestFromCLI([]string{"--task", "继续当前任务"}, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if request.Session.SessionID == nil {
		t.Fatalf("expected session id")
	}
	if *request.Session.SessionID == "corrupt-session" {
		t.Fatalf("session id should not reuse corrupt record pin")
	}
}

func TestWorkspaceDiscoveryKeepsTaskPinAcrossSubdirectories(t *testing.T) {
	setTestMiraHome(t)
	root := t.TempDir()
	subA := filepath.Join(root, "a")
	subB := filepath.Join(root, "b")
	if err := os.MkdirAll(subA, 0o755); err != nil {
		t.Fatalf("mkdir a: %v", err)
	}
	if err := os.MkdirAll(subB, 0o755); err != nil {
		t.Fatalf("mkdir b: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module mira/test\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	first, err := LoadRequestFromCLI([]string{"--task", "审阅主方案"}, subA)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	second, err := LoadRequestFromCLI([]string{"--task", "补充看看依赖材料"}, subB)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if first.Session.SessionID == nil || second.Session.SessionID == nil {
		t.Fatalf("expected session ids")
	}
	if *first.Session.SessionID != *second.Session.SessionID {
		t.Fatalf("session ids differ across subdirectories: %s vs %s", *first.Session.SessionID, *second.Session.SessionID)
	}
}

func TestMissingPinRebuildsDeterministicWorkspaceSession(t *testing.T) {
	setTestMiraHome(t)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module mira/test\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	firstPath := filepath.Join(root, "context-a.md")
	secondPath := filepath.Join(root, "context-b.md")
	if err := os.WriteFile(firstPath, []byte("main doc"), 0o644); err != nil {
		t.Fatalf("write context a: %v", err)
	}
	if err := os.WriteFile(secondPath, []byte("extra material"), 0o644); err != nil {
		t.Fatalf("write context b: %v", err)
	}
	first, err := LoadRequestFromCLI([]string{"--context-file", firstPath, "--task", "审阅主文档"}, root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pinPaths, err := filepath.Glob(filepath.Join(os.Getenv("MIRA_HOME"), "session_pins", "*.json"))
	if err != nil || len(pinPaths) != 1 {
		t.Fatalf("pin paths = %v, err = %v", pinPaths, err)
	}
	if err := os.Remove(pinPaths[0]); err != nil {
		t.Fatalf("remove pin: %v", err)
	}
	second, err := LoadRequestFromCLI([]string{"--context-file", secondPath, "--task", "补充看看 QA 规范"}, root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if first.Session.SessionID == nil || second.Session.SessionID == nil {
		t.Fatalf("expected session ids")
	}
	if *first.Session.SessionID != *second.Session.SessionID {
		t.Fatalf("deterministic fallback failed: %s vs %s", *first.Session.SessionID, *second.Session.SessionID)
	}
}

func TestExplicitEphemeralDisablesAutoSticky(t *testing.T) {
	setTestMiraHome(t)
	request, err := LoadRequestFromCLI([]string{"--ephemeral", "--task", "Reply with OK only"}, "/tmp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if request.Session.Mode != "ephemeral" {
		t.Fatalf("session mode = %s, want ephemeral", request.Session.Mode)
	}
	if request.Session.SessionID != nil {
		t.Fatalf("session id = %v, want nil", *request.Session.SessionID)
	}
}

func TestRejectsSessionAndNewSessionTogether(t *testing.T) {
	setTestMiraHome(t)
	_, err := LoadRequestFromCLI([]string{"--session", "demo", "--new-session", "--task", "demo"}, "/tmp")
	if err == nil || err.Error() != "cannot combine --session with --new-session" {
		t.Fatalf("error = %v", err)
	}
}

func TestExplicitSessionOverridesEnvSessionID(t *testing.T) {
	setTestMiraHome(t)
	t.Setenv("MIRA_SESSION_ID", "env-session")
	request, err := LoadRequestFromCLI([]string{"--session", "flag-session", "--task", "demo"}, "/tmp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if request.Session.SessionID == nil || *request.Session.SessionID != "flag-session" {
		t.Fatalf("session id = %v, want flag-session", request.Session.SessionID)
	}
}

func TestRejectsNewSessionAndEphemeralTogether(t *testing.T) {
	setTestMiraHome(t)
	_, err := LoadRequestFromCLI([]string{"--new-session", "--ephemeral", "--task", "demo"}, "/tmp")
	if err == nil || err.Error() != "cannot combine --new-session with --ephemeral" {
		t.Fatalf("error = %v", err)
	}
}

func TestInputFileRejectsEphemeralOverride(t *testing.T) {
	setTestMiraHome(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "request.json")
	if err := os.WriteFile(path, []byte(`{"version":"v1","task":"demo","context":{"diff":"","files":[],"docs":[]}}`), 0o644); err != nil {
		t.Fatalf("write error: %v", err)
	}
	_, err := LoadRequestFromCLI([]string{"--input-file", path, "--ephemeral"}, dir)
	if err == nil || err.Error() != "session/context-file/file/request-id/task-description/git-branch/ephemeral/new-session are not supported with --input-file" {
		t.Fatalf("error = %v", err)
	}
}

func TestInputFileRejectsNewSessionOverride(t *testing.T) {
	setTestMiraHome(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "request.json")
	if err := os.WriteFile(path, []byte(`{"version":"v1","task":"demo","context":{"diff":"","files":[],"docs":[]}}`), 0o644); err != nil {
		t.Fatalf("write error: %v", err)
	}
	_, err := LoadRequestFromCLI([]string{"--input-file", path, "--new-session"}, dir)
	if err == nil || err.Error() != "session/context-file/file/request-id/task-description/git-branch/ephemeral/new-session are not supported with --input-file" {
		t.Fatalf("error = %v", err)
	}
}

func stringPtr(value string) *string {
	return &value
}
