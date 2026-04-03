package contract

import "fmt"

func validateTypedSession(session Session) (Session, error) {
	mode := session.Mode
	if mode == "" {
		mode = "ephemeral"
	}
	if mode != "sticky" && mode != "ephemeral" {
		return Session{}, fmt.Errorf("unsupported session mode: %s", mode)
	}
	sessionID, err := validateOptionalField(session.SessionID, "session.session_id")
	if err != nil {
		return Session{}, err
	}
	if mode == "sticky" && sessionID == nil {
		return Session{}, fmt.Errorf("sticky session requires session.session_id")
	}
	parentID, err := validateOptionalField(session.ParentSessionID, "session.parent_session_id")
	if err != nil {
		return Session{}, err
	}
	contextHint, err := validateTypedContextHint(session.ContextHint)
	if err != nil {
		return Session{}, err
	}
	return Session{Mode: mode, SessionID: sessionID, ParentSessionID: parentID, ContextHint: contextHint}, nil
}

func validateTypedContextHint(hint SessionContextHint) (SessionContextHint, error) {
	workspaceRoot, err := validateOptionalField(hint.WorkspaceRoot, "session.context_hint.workspace_root")
	if err != nil {
		return SessionContextHint{}, err
	}
	taskDescription, err := validateOptionalField(hint.TaskDescription, "session.context_hint.task_description")
	if err != nil {
		return SessionContextHint{}, err
	}
	gitBranch, err := validateOptionalField(hint.GitBranch, "session.context_hint.git_branch")
	if err != nil {
		return SessionContextHint{}, err
	}
	return SessionContextHint{
		WorkspaceRoot:   workspaceRoot,
		TaskDescription: taskDescription,
		GitBranch:       gitBranch,
	}, nil
}

func validateTypedManifest(manifest FileManifest) (FileManifest, error) {
	mode := manifest.Mode
	if mode == "" {
		mode = "none"
	}
	if mode != "none" && mode != "explicit" {
		return FileManifest{}, fmt.Errorf("unsupported file_manifest mode: %s", mode)
	}
	paths, err := validateTypedStringSlice(manifest.Paths, "file_manifest.paths")
	if err != nil {
		return FileManifest{}, err
	}
	maxTotalBytes := manifest.MaxTotalBytes
	if maxTotalBytes == 0 {
		maxTotalBytes = DefaultMaxFileSize
	}
	if maxTotalBytes < 1 || maxTotalBytes > 1<<30 {
		return FileManifest{}, fmt.Errorf("file_manifest.max_total_bytes must be an integer between 1 and %d", 1<<30)
	}
	return FileManifest{Mode: mode, Paths: paths, MaxTotalBytes: maxTotalBytes, ReadOnly: true}, nil
}

func validateTypedContext(context Context) (Context, error) {
	files, err := validateTypedFiles(context.Files)
	if err != nil {
		return Context{}, err
	}
	docs, err := validateTypedDocs(context.Docs)
	if err != nil {
		return Context{}, err
	}
	return Context{Diff: context.Diff, Files: files, Docs: docs}, nil
}

func validateTypedFiles(items []ContextFile) ([]ContextFile, error) {
	if len(items) == 0 {
		return []ContextFile{}, nil
	}
	result := make([]ContextFile, 0, len(items))
	for index, item := range items {
		if item.Path == "" {
			return nil, fmt.Errorf("context.files[%d].path must be a non-empty string", index)
		}
		if item.Content == "" {
			return nil, fmt.Errorf("context.files[%d].content must be a non-empty string", index)
		}
		source, err := validateOptionalField(item.Source, fmt.Sprintf("context.files[%d].source", index))
		if err != nil {
			return nil, err
		}
		title, err := validateOptionalField(item.Title, fmt.Sprintf("context.files[%d].title", index))
		if err != nil {
			return nil, err
		}
		result = append(result, ContextFile{Path: item.Path, Content: item.Content, Source: source, Title: title})
	}
	return result, nil
}

func validateTypedDocs(items []ContextDoc) ([]ContextDoc, error) {
	if len(items) == 0 {
		return []ContextDoc{}, nil
	}
	result := make([]ContextDoc, 0, len(items))
	for index, item := range items {
		if item.Content == "" {
			return nil, fmt.Errorf("context.docs[%d].content must be a non-empty string", index)
		}
		source, err := validateOptionalField(item.Source, fmt.Sprintf("context.docs[%d].source", index))
		if err != nil {
			return nil, err
		}
		title, err := validateOptionalField(item.Title, fmt.Sprintf("context.docs[%d].title", index))
		if err != nil {
			return nil, err
		}
		result = append(result, ContextDoc{Content: item.Content, Source: source, Title: title})
	}
	return result, nil
}

func validateTypedMaxTokens(value int) (int, error) {
	if value == 0 {
		return DefaultMaxTokens, nil
	}
	if value < 256 || value > MaxMaxTokens {
		return 0, fmt.Errorf("max_tokens must be an integer between 256 and %d", MaxMaxTokens)
	}
	return value, nil
}

func validatePromptOverrides(overrides *PromptOverrides) (*PromptOverrides, error) {
	if overrides == nil {
		return nil, nil
	}
	protocol, err := validateOptionalField(overrides.Protocol, "prompt_overrides.protocol")
	if err != nil {
		return nil, err
	}
	output, err := validateOptionalField(overrides.OutputInstructions, "prompt_overrides.output_instructions")
	if err != nil {
		return nil, err
	}
	if protocol == nil && output == nil {
		return nil, nil
	}
	return &PromptOverrides{Protocol: protocol, OutputInstructions: output}, nil
}
