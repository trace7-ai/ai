package contract

import (
	"fmt"

	"mira/pkg/roles"
)

func ValidateRequest(request Request) (Request, error) {
	version, err := validateVersion(request.Version)
	if err != nil {
		return Request{}, err
	}
	roleName, err := roles.Normalize(request.Role)
	if err != nil {
		return Request{}, err
	}
	contentFormat, err := roles.ResolveContentFormat(request.ContentFormat)
	if err != nil {
		return Request{}, err
	}
	requestID, err := validateOptionalField(request.RequestID, "request_id")
	if err != nil {
		return Request{}, err
	}
	session, err := validateTypedSession(request.Session)
	if err != nil {
		return Request{}, err
	}
	manifest, err := validateTypedManifest(request.FileManifest)
	if err != nil {
		return Request{}, err
	}
	constraints, err := validateTypedStringSlice(request.Constraints, "constraints")
	if err != nil {
		return Request{}, err
	}
	context, err := validateTypedContext(request.Context)
	if err != nil {
		return Request{}, err
	}
	promptOverrides, err := validatePromptOverrides(request.PromptOverrides)
	if err != nil {
		return Request{}, err
	}
	maxTokens, err := validateTypedMaxTokens(request.MaxTokens)
	if err != nil {
		return Request{}, err
	}
	timeoutSec, err := validateTimeout(request.TimeoutSec)
	if err != nil {
		return Request{}, err
	}
	if request.Task == "" {
		return Request{}, fmt.Errorf("task must be a non-empty string")
	}
	if manifest.Mode == "explicit" && session.ContextHint.WorkspaceRoot == nil {
		return Request{}, fmt.Errorf("explicit file_manifest requires session.context_hint.workspace_root")
	}
	return Request{
		Version:         version,
		Role:            roleName,
		RequestID:       requestID,
		ContentFormat:   contentFormat,
		Session:         session,
		FileManifest:    manifest,
		Task:            request.Task,
		Constraints:     constraints,
		Context:         context,
		PromptOverrides: promptOverrides,
		MaxTokens:       maxTokens,
		TimeoutSec:      timeoutSec,
	}, nil
}

func validateVersion(version string) (string, error) {
	if version == "" {
		return SchemaVersion, nil
	}
	if version != SchemaVersion {
		return "", fmt.Errorf("unsupported version: %s", version)
	}
	return SchemaVersion, nil
}
