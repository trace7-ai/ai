package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"mira/pkg/contract"
	"mira/pkg/routing"
)

func LoadRequestFromCLI(args []string, cwd string) (contract.Request, error) {
	parsed, err := ParseAskArgs(args)
	if err != nil {
		return contract.Request{}, err
	}
	if parsed.InputFile != "" {
		return loadRequestFileMode(parsed)
	}
	return buildPromptModeRequest(parsed, cwd)
}

func loadRequestFileMode(parsed ParsedArgs) (contract.Request, error) {
	if err := rejectFileModeOverrides(parsed); err != nil {
		return contract.Request{}, err
	}
	raw, err := os.ReadFile(parsed.InputFile)
	if err != nil {
		return contract.Request{}, err
	}
	body, err := contract.DecodeRequestBody(raw)
	if err != nil {
		return contract.Request{}, err
	}
	if parsed.Role != "" {
		body["role"] = parsed.Role
	}
	if parsed.ContentFormat != "auto" {
		body["content_format"] = parsed.ContentFormat
	}
	return contract.NormalizeRequest(body)
}

func buildPromptModeRequest(parsed ParsedArgs, cwd string) (contract.Request, error) {
	context, err := loadContextPayload(parsed.ContextFile)
	if err != nil {
		return contract.Request{}, err
	}
	roleName := parsed.Role
	if roleName == "" {
		roleName = routing.InferRole(
			parsed.InlineTask,
			context["diff"].(string),
			parsed.Files,
			len(emptyAnySlice(context["docs"])) > 0,
		)
	}
	taskDescription := parsed.TaskDesc
	if taskDescription == "" {
		taskDescription = parsed.InlineTask
	}
	raw := map[string]any{
		"version":        contract.SchemaVersion,
		"role":           roleName,
		"request_id":     emptyToNil(parsed.RequestID),
		"content_format": parsed.ContentFormat,
		"session": map[string]any{
			"mode":       sessionMode(parsed.Session),
			"session_id": emptyToNil(parsed.Session),
			"context_hint": map[string]any{
				"workspace_root":   resolveWorkspaceRoot(parsed.WorkspaceRoot, cwd),
				"task_description": taskDescription,
				"git_branch":       emptyToNil(parsed.GitBranch),
			},
		},
		"file_manifest": map[string]any{
			"mode":            manifestMode(parsed.Files),
			"paths":           toAnySlice(parsed.Files),
			"max_total_bytes": contract.DefaultMaxFileSize,
			"read_only":       true,
		},
		"task":        parsed.InlineTask,
		"constraints": []any{},
		"context":     context,
		"max_tokens":  parsed.MaxTokens,
		"timeout_sec": parsed.TimeoutSec,
	}
	return contract.NormalizeRequest(raw)
}

func loadContextPayload(rawPath string) (map[string]any, error) {
	if rawPath == "" {
		return map[string]any{"diff": "", "files": []any{}, "docs": []any{}}, nil
	}
	path, err := filepath.Abs(rawPath)
	if err != nil {
		return nil, err
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if filepath.Ext(path) == ".json" {
		return loadJSONContextPayload(body, path)
	}
	return map[string]any{
		"diff":  "",
		"files": []any{},
		"docs":  []any{map[string]any{"source": path, "content": string(body)}},
	}, nil
}

func loadJSONContextPayload(body []byte, path string) (map[string]any, error) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("context file is not valid JSON: %s", path)
	}
	return normalizeContextPayload(payload, path)
}
