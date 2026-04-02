package cli

import (
	"fmt"
	"path/filepath"
)

func rejectFileModeOverrides(parsed ParsedArgs) error {
	if parsed.Session != "" || parsed.ContextFile != "" || len(parsed.Files) > 0 || parsed.RequestID != "" || parsed.TaskDesc != "" || parsed.GitBranch != "" {
		return fmt.Errorf("session/context-file/file/request-id/task-description/git-branch are not supported with --input-file")
	}
	if parsed.WorkspaceRoot != "" {
		return fmt.Errorf("workspace-root is not supported with --input-file")
	}
	if parsed.TimeoutSec != 180 {
		return fmt.Errorf("timeout-sec is not supported with --input-file")
	}
	if parsed.MaxTokens != 4000 {
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

func sessionMode(session string) string {
	if session == "" {
		return "ephemeral"
	}
	return "sticky"
}

func resolveWorkspaceRoot(rawPath, cwd string) string {
	if rawPath == "" {
		return cwd
	}
	path, err := filepath.Abs(rawPath)
	if err != nil {
		return rawPath
	}
	return path
}

func emptyToNil(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func toAnySlice(items []string) []any {
	result := make([]any, 0, len(items))
	for _, item := range items {
		result = append(result, item)
	}
	return result
}
