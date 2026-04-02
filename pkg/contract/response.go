package contract

func BuildErrorResponse(code, message string, role, requestID, model *string) Response {
	return Response{
		Version:     SchemaVersion,
		Status:      "error",
		Role:        role,
		RequestID:   requestID,
		Model:       model,
		ContentType: nil,
		Result:      nil,
		Errors:      []ErrorItem{{Code: code, Message: message}},
		TokenUsage:  TokenUsage{Input: nil, Output: nil},
		Truncated:   false,
		FilesRead:   []map[string]any{},
		Session:     nil,
	}
}

func BuildSuccessResponse(role string, requestID, model *string, result any, contentType string) Response {
	return Response{
		Version:     SchemaVersion,
		Status:      "ok",
		Role:        stringPtr(role),
		RequestID:   requestID,
		Model:       model,
		ContentType: stringPtr(contentType),
		Result:      result,
		Errors:      []ErrorItem{},
		TokenUsage:  TokenUsage{Input: nil, Output: nil},
		Truncated:   false,
		FilesRead:   []map[string]any{},
		Session:     nil,
	}
}
