package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mira/pkg/contract"
	"mira/pkg/session"
)

const (
	autoSessionPrefix     = "auto"
	autoSessionHashLength = 12
)

func rejectFileModeOverrides(parsed ParsedArgs) error {
	if parsed.Session != "" || parsed.Ephemeral || parsed.NewSession || parsed.ContextFile != "" || len(parsed.Files) > 0 || parsed.RequestID != "" || parsed.TaskDesc != "" || parsed.GitBranch != "" {
		return fmt.Errorf("session/context-file/file/request-id/task-description/git-branch/ephemeral/new-session are not supported with --input-file")
	}
	if parsed.WorkspaceRoot != "" {
		return fmt.Errorf("workspace-root is not supported with --input-file")
	}
	if parsed.TimeoutSec != contract.DefaultTimeoutSec {
		return fmt.Errorf("timeout-sec is not supported with --input-file")
	}
	if parsed.MaxTokens != contract.DefaultMaxTokens {
		return fmt.Errorf("max-tokens is not supported with --input-file")
	}
	return nil
}

func normalizeContextPayload(payload map[string]any, path string) (map[string]any, error) {
	body := payload
	if nested, ok := body["context"]; ok {
		contextBody, ok := nested.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("context field must be an object: %s", path)
		}
		body = contextBody
	}
	return map[string]any{
		"diff":  body["diff"],
		"files": emptyAnySlice(body["files"]),
		"docs":  emptyAnySlice(body["docs"]),
	}, nil
}

func emptyAnySlice(raw any) []any {
	items, ok := raw.([]any)
	if !ok || items == nil {
		return []any{}
	}
	return items
}

func manifestMode(paths []string) string {
	if len(paths) == 0 {
		return "none"
	}
	return "explicit"
}

func resolvePromptSession(parsed ParsedArgs, cwd string) (string, any, error) {
	if parsed.Ephemeral {
		return "ephemeral", nil, nil
	}
	workspaceRoot := resolveWorkspaceRoot(parsed.WorkspaceRoot, cwd)
	taskDescription := optionalTaskDescription(firstNonEmpty(parsed.TaskDesc, parsed.InlineTask))
	store, err := session.New("")
	if err != nil {
		return "", nil, err
	}
	unlock, err := store.LockTaskPin(workspaceRoot)
	if err != nil {
		return "", nil, err
	}
	defer unlock()
	if parsed.Session != "" {
		return "sticky", parsed.Session, store.SaveTaskPin(workspaceRoot, parsed.Session, taskDescription)
	}
	if parsed.NewSession {
		sessionID := freshSessionID(parsed, cwd)
		return "sticky", sessionID, store.SaveTaskPin(workspaceRoot, sessionID, taskDescription)
	}
	if sessionID := envSessionID(); sessionID != "" {
		return "sticky", sessionID, store.SaveTaskPin(workspaceRoot, sessionID, taskDescription)
	}
	pin, err := store.LoadTaskPin(workspaceRoot)
	if err != nil {
		return "", nil, err
	}
	if pin != nil && taskPinReusable(store, pin.SessionID) {
		return "sticky", pin.SessionID, store.SaveTaskPin(workspaceRoot, pin.SessionID, taskDescription)
	}
	sessionID := autoSessionID(parsed, cwd)
	if err := store.SaveTaskPin(workspaceRoot, sessionID, taskDescription); err != nil {
		return "", nil, err
	}
	return "sticky", sessionID, nil
}

func autoSessionID(parsed ParsedArgs, cwd string) string {
	seed := resolveWorkspaceRoot(parsed.WorkspaceRoot, cwd)
	return autoSessionPrefix + "-" + shortSessionHash(seed)
}

func freshSessionID(parsed ParsedArgs, cwd string) string {
	seed := strings.Join([]string{
		resolveWorkspaceRoot(parsed.WorkspaceRoot, cwd),
		sessionTaskClass(parsed),
		firstNonEmpty(parsed.TaskDesc, parsed.InlineTask),
		time.Now().UTC().Format(time.RFC3339Nano),
	}, "\n")
	return autoSessionPrefix + "-" + shortSessionHash(seed)
}

func sessionTaskClass(parsed ParsedArgs) string {
	text := strings.ToLower(strings.Join(strings.Fields(parsed.TaskDesc+" "+parsed.InlineTask), " "))
	switch {
	case containsAny(text, "review", "审阅", "评审", "reviewer"):
		return "review"
	case containsAny(text, "summary", "summarize", "总结", "摘要"):
		return "summarize"
	case containsAny(text, "plan", "规划", "计划", "方案"):
		return "plan"
	default:
		return "ask"
	}
}

func containsAny(text string, values ...string) bool {
	for _, value := range values {
		if strings.Contains(text, value) {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func optionalTaskDescription(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func taskPinReusable(store *session.Store, sessionID string) bool {
	record, err := store.Load(sessionID)
	if err != nil {
		return false
	}
	if record == nil {
		return true
	}
	return record.Status != session.InvalidStatus
}

func shortSessionHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])[:autoSessionHashLength]
}

func resolveWorkspaceRoot(rawPath, cwd string) string {
	if rawPath == "" {
		return discoverWorkspaceRoot(cwd)
	}
	path, err := filepath.Abs(rawPath)
	if err != nil {
		return rawPath
	}
	return path
}

func discoverWorkspaceRoot(cwd string) string {
	path, err := filepath.Abs(cwd)
	if err != nil {
		return cwd
	}
	for current := path; ; current = filepath.Dir(current) {
		if hasWorkspaceMarker(current) {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			return path
		}
	}
}

func hasWorkspaceMarker(path string) bool {
	for _, marker := range []string{"go.mod", ".git", ".codex", "AGENTS.md"} {
		if _, err := os.Stat(filepath.Join(path, marker)); err == nil {
			return true
		}
	}
	return false
}

func emptyToNil(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func envSessionID() string {
	return strings.TrimSpace(os.Getenv("MIRA_SESSION_ID"))
}

func toAnySlice(items []string) []any {
	result := make([]any, 0, len(items))
	for _, item := range items {
		result = append(result, item)
	}
	return result
}
