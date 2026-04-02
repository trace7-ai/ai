package runner

import (
	"context"
	"errors"
	"strings"

	"mira/pkg/contract"
	"mira/pkg/prompt"
	"mira/pkg/roles"
	"mira/pkg/transport"
)

const (
	ExitOK             = 0
	ExitExecutionError = 1
	ExitInvalidRequest = 2
	ExitTimeout        = 3
)

func ExecuteRequest(ctx context.Context, request contract.Request, client transport.Transport) (contract.Response, int) {
	role, ok := roles.Get(request.Role)
	if !ok {
		return contract.BuildErrorResponse("invalid_request", "unsupported role: "+request.Role, &request.Role, request.RequestID, nil), ExitInvalidRequest
	}
	modelName := clientModelName(client)
	stream, err := client.Execute(ctx, prompt.Build(request, role), transport.Options{
		MaxTokens:  request.MaxTokens,
		TimeoutSec: request.TimeoutSec,
		RequestID:  request.RequestID,
		SessionID:  request.Session.SessionID,
	})
	if err != nil {
		code, exitCode := executionErrorDetails(err)
		return contract.BuildErrorResponse(code, err.Error(), &request.Role, request.RequestID, modelName), exitCode
	}
	defer stream.Close()
	text, usage, err := collectStream(ctx, stream)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return contract.BuildErrorResponse("timeout", err.Error(), &request.Role, request.RequestID, modelName), ExitTimeout
		}
		code, exitCode := executionErrorDetails(err)
		return contract.BuildErrorResponse(code, err.Error(), &request.Role, request.RequestID, modelName), exitCode
	}
	result, err := ParseModelResult(text, role, request.ContentFormat)
	if err != nil {
		return contract.BuildErrorResponse("execution_failed", err.Error(), &request.Role, request.RequestID, modelName), ExitExecutionError
	}
	response := contract.BuildSuccessResponse(request.Role, request.RequestID, modelName, result, request.ContentFormat)
	response.TokenUsage = usage
	return response, ExitOK
}

func executionErrorDetails(err error) (string, int) {
	if errors.Is(err, transport.ErrNotImplemented) {
		return "not_implemented", ExitExecutionError
	}
	if isSessionError(err.Error()) {
		return "invalid_session", ExitInvalidRequest
	}
	return "execution_failed", ExitExecutionError
}

func isSessionError(message string) bool {
	lowered := strings.ToLower(message)
	hints := []string{
		"invalid session",
		"session expired",
		"session not found",
		"conversation not found",
		"invalid conversation",
		"会话失效",
		"会话不存在",
		"会话已过期",
	}
	for _, hint := range hints {
		if strings.Contains(lowered, hint) {
			return true
		}
	}
	return false
}

func clientModelName(client transport.Transport) *string {
	reporter, ok := client.(transport.ModelAware)
	if !ok {
		return nil
	}
	return reporter.ModelName()
}
