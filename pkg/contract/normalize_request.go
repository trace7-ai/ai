package contract

import (
	"fmt"

	"mira/pkg/roles"
)

func NormalizeRequest(raw map[string]any) (Request, error) {
	version := SchemaVersion
	if value, ok := raw["version"]; ok {
		text, err := requireString(value, "version")
		if err != nil {
			return Request{}, err
		}
		version = text
	}
	if version != SchemaVersion {
		return Request{}, fmt.Errorf("unsupported version: %s", version)
	}
	roleName, err := requireString(raw["role"], "role")
	if err != nil {
		return Request{}, err
	}
	role, ok := roles.Get(roleName)
	if !ok {
		return Request{}, fmt.Errorf("unsupported role: %s", roleName)
	}
	task, err := requireString(raw["task"], "task")
	if err != nil {
		return Request{}, err
	}
	requestID, err := optionalString(raw["request_id"], "request_id")
	if err != nil {
		return Request{}, err
	}
	contentFormat, err := normalizeContentFormat(raw["content_format"], role)
	if err != nil {
		return Request{}, err
	}
	session, err := normalizeSession(raw["session"])
	if err != nil {
		return Request{}, err
	}
	manifest, err := normalizeFileManifest(raw["file_manifest"])
	if err != nil {
		return Request{}, err
	}
	constraints, err := toStringSlice(raw["constraints"], "constraints")
	if err != nil {
		return Request{}, err
	}
	context, err := normalizeContext(raw["context"])
	if err != nil {
		return Request{}, err
	}
	maxTokens, err := normalizeInt(raw["max_tokens"], DefaultMaxTokens, 256, MaxMaxTokens, "max_tokens")
	if err != nil {
		return Request{}, err
	}
	timeoutSec, err := normalizeInt(raw["timeout_sec"], DefaultTimeoutSec, 10, MaxTimeoutSec, "timeout_sec")
	if err != nil {
		return Request{}, err
	}
	if manifest.Mode == "explicit" && session.ContextHint.WorkspaceRoot == nil {
		return Request{}, fmt.Errorf("explicit file_manifest requires session.context_hint.workspace_root")
	}
	return Request{
		Version:       SchemaVersion,
		Role:          role.Name,
		RequestID:     requestID,
		ContentFormat: contentFormat,
		Session:       session,
		FileManifest:  manifest,
		Task:          task,
		Constraints:   constraints,
		Context:       context,
		MaxTokens:     maxTokens,
		TimeoutSec:    timeoutSec,
	}, nil
}

func normalizeContentFormat(raw any, role roles.Spec) (string, error) {
	if raw == nil {
		return role.ResolveContentFormat("auto")
	}
	value, err := requireString(raw, "content_format")
	if err != nil {
		return "", err
	}
	return role.ResolveContentFormat(value)
}

func normalizeSession(raw any) (Session, error) {
	body, err := normalizeOptionalObject(raw, "session")
	if err != nil {
		return Session{}, err
	}
	mode := "ephemeral"
	if value, ok := body["mode"]; ok {
		text, err := requireString(value, "session.mode")
		if err != nil {
			return Session{}, err
		}
		mode = text
	}
	if mode != "sticky" && mode != "ephemeral" {
		return Session{}, fmt.Errorf("unsupported session mode: %s", mode)
	}
	sessionID, err := optionalString(body["session_id"], "session.session_id")
	if err != nil {
		return Session{}, err
	}
	if mode == "sticky" && sessionID == nil {
		return Session{}, fmt.Errorf("sticky session requires session.session_id")
	}
	parentID, err := optionalString(body["parent_session_id"], "session.parent_session_id")
	if err != nil {
		return Session{}, err
	}
	contextHint, err := normalizeContextHint(body["context_hint"])
	if err != nil {
		return Session{}, err
	}
	return Session{Mode: mode, SessionID: sessionID, ParentSessionID: parentID, ContextHint: contextHint}, nil
}

func normalizeFileManifest(raw any) (FileManifest, error) {
	body, err := normalizeOptionalObject(raw, "file_manifest")
	if err != nil {
		return FileManifest{}, err
	}
	mode := "none"
	if value, ok := body["mode"]; ok {
		text, err := requireString(value, "file_manifest.mode")
		if err != nil {
			return FileManifest{}, err
		}
		mode = text
	}
	if mode != "none" && mode != "explicit" {
		return FileManifest{}, fmt.Errorf("unsupported file_manifest mode: %s", mode)
	}
	paths, err := toStringSlice(body["paths"], "file_manifest.paths")
	if err != nil {
		return FileManifest{}, err
	}
	maxTotalBytes, err := normalizeInt(body["max_total_bytes"], DefaultMaxFileSize, 1, 1<<30, "file_manifest.max_total_bytes")
	if err != nil {
		return FileManifest{}, err
	}
	return FileManifest{Mode: mode, Paths: paths, MaxTotalBytes: maxTotalBytes, ReadOnly: true}, nil
}
