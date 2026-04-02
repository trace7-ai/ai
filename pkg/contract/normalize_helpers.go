package contract

import "fmt"

func normalizeOptionalObject(raw any, field string) (map[string]any, error) {
	if raw == nil {
		return map[string]any{}, nil
	}
	return requireObject(raw, field)
}

func normalizeContextHint(raw any) (SessionContextHint, error) {
	body, err := normalizeOptionalObject(raw, "session.context_hint")
	if err != nil {
		return SessionContextHint{}, err
	}
	workspaceRoot, err := optionalString(body["workspace_root"], "session.context_hint.workspace_root")
	if err != nil {
		return SessionContextHint{}, err
	}
	taskDescription, err := optionalString(body["task_description"], "session.context_hint.task_description")
	if err != nil {
		return SessionContextHint{}, err
	}
	gitBranch, err := optionalString(body["git_branch"], "session.context_hint.git_branch")
	if err != nil {
		return SessionContextHint{}, err
	}
	return SessionContextHint{
		WorkspaceRoot:   workspaceRoot,
		TaskDescription: taskDescription,
		GitBranch:       gitBranch,
	}, nil
}

func normalizeInt(raw any, defaultValue, min, max int, field string) (int, error) {
	if raw == nil {
		return defaultValue, nil
	}
	switch value := raw.(type) {
	case int:
		if value < min || value > max {
			return 0, fmt.Errorf("%s must be an integer between %d and %d", field, min, max)
		}
		return value, nil
	case float64:
		result := int(value)
		if float64(result) != value || result < min || result > max {
			return 0, fmt.Errorf("%s must be an integer between %d and %d", field, min, max)
		}
		return result, nil
	default:
		return 0, fmt.Errorf("%s must be an integer between %d and %d", field, min, max)
	}
}
