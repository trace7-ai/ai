package runner

import (
	"context"
	"errors"
	"strings"

	"mira/pkg/contract"
	"mira/pkg/prompt"
	"mira/pkg/transport"
)

const (
	ExitOK             = 0
	ExitExecutionError = 1
	ExitInvalidRequest = 2
	ExitTimeout        = 3
)

func ExecuteRequest(ctx context.Context, request contract.Request, client transport.Transport, onChunk func([]byte)) (contract.Response, int) {
	modelName := clientModelName(client)
	promptPayload := prompt.Build(request)
	stream, err := client.Execute(ctx, promptPayload, transport.Options{
		MaxTokens:  request.MaxTokens,
		TimeoutSec: request.TimeoutSec,
		RequestID:  request.RequestID,
		SessionID:  request.Session.SessionID,
	})
	if err != nil {
		code, exitCode := executionErrorDetails(err)
		response := contract.BuildErrorResponse(code, err.Error(), &request.Role, request.RequestID, modelName)
		response.PromptMeta = promptPayload.Meta
		return response, exitCode
	}
	defer stream.Close()
	text, usage, truncated, err := collectStream(ctx, stream, onChunk)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			response := contract.BuildErrorResponse("timeout", err.Error(), &request.Role, request.RequestID, modelName)
			response.PromptMeta = promptPayload.Meta
			return response, ExitTimeout
		}
		code, exitCode := executionErrorDetails(err)
		response := contract.BuildErrorResponse(code, err.Error(), &request.Role, request.RequestID, modelName)
		response.PromptMeta = promptPayload.Meta
		return response, exitCode
	}
	result, contentType, err := ParseModelResult(text, request.ContentFormat)
	if err != nil {
		response := contract.BuildErrorResponse("execution_failed", err.Error(), &request.Role, request.RequestID, modelName)
		response.PromptMeta = promptPayload.Meta
		return response, ExitExecutionError
	}
	response := contract.BuildSuccessResponse(request.Role, request.RequestID, modelName, result, contentType)
	response.TokenUsage = usage
	response.Truncated = truncated
	response.PromptMeta = promptPayload.Meta
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
