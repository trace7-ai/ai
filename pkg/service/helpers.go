package service

import (
	"mira/pkg/contract"
	"mira/pkg/fileaccess"
	"mira/pkg/runner"
	"mira/pkg/session"
	"mira/pkg/transport"
)

func hydrateRequest(request contract.Request) (contract.Request, []map[string]any, error) {
	accessor, err := fileaccess.New(request.FileManifest, derefString(request.Session.ContextHint.WorkspaceRoot))
	if err != nil {
		return contract.Request{}, nil, err
	}
	files, filesRead, err := accessor.ReadAuthorizedFiles()
	if err != nil {
		return contract.Request{}, nil, err
	}
	request.Context.Files = append(request.Context.Files, files...)
	return request, filesRead, nil
}

func rejectSession(request contract.Request, snapshot session.Snapshot, modelAware transport.ModelAware) (*contract.Response, int) {
	model := modelName(modelAware)
	if snapshot.Status == session.InvalidStatus {
		response := contract.BuildErrorResponse("invalid_session", reason(snapshot.Reason), stringPtr(request.Role), request.RequestID, model)
		response.Session = sessionPayload(*request.Session.SessionID, snapshot, sessionWasReused(snapshot))
		return &response, runner.ExitInvalidRequest
	}
	if snapshot.Status == session.ActiveStatus && snapshot.Record != nil {
		if mismatch := storeCompatibility(snapshot.Record, request); mismatch != nil {
			response := contract.BuildErrorResponse("session_incompatible", *mismatch, stringPtr(request.Role), request.RequestID, model)
			response.Session = sessionPayload(*request.Session.SessionID, snapshot, sessionWasReused(snapshot))
			return &response, runner.ExitInvalidRequest
		}
	}
	return nil, 0
}

func finalizeStickyResponse(service Service, request contract.Request, snapshot session.Snapshot, response contract.Response, exitCode int) (contract.Response, int, error) {
	if request.Session.SessionID == nil {
		return response, exitCode, nil
	}
	if response.Status != "ok" {
		if len(response.Errors) > 0 && response.Errors[0].Code == "invalid_session" && snapshot.Record != nil {
			invalid, err := service.Store.MarkInvalid(*request.Session.SessionID, *snapshot.Record, response.Errors[0].Message)
			if err != nil {
				return contract.Response{}, 0, err
			}
			response.Session = sessionPayload(*request.Session.SessionID, session.Snapshot{Status: invalid.Status, Record: &invalid, Reason: invalid.LastError}, true)
			return response, runner.ExitInvalidRequest, nil
		}
		response.Session = sessionPayload(*request.Session.SessionID, session.Snapshot{Status: "error", Record: snapshot.Record, Reason: snapshot.Reason}, sessionWasReused(snapshot))
		return response, exitCode, nil
	}
	sessionAware, ok := service.Client.(transport.SessionAware)
	if !ok || sessionAware.RemoteSessionID() == "" {
		response.Session = sessionPayload(*request.Session.SessionID, snapshot, sessionWasReused(snapshot))
		return response, exitCode, nil
	}
	record, err := session.BuildRecord(request, sessionAware.RemoteSessionID(), snapshot.Record)
	if err != nil {
		return contract.Response{}, 0, err
	}
	if err := service.Store.Save(*request.Session.SessionID, record); err != nil {
		return contract.Response{}, 0, err
	}
	response.Session = sessionPayload(*request.Session.SessionID, session.Snapshot{Status: record.Status, Record: &record, Reason: record.LastError}, sessionWasReused(snapshot))
	return response, exitCode, nil
}

func sessionPayload(sessionID string, snapshot session.Snapshot, reused bool) *contract.SessionPayload {
	payload := &contract.SessionPayload{
		SessionID: sessionID,
		Status:    snapshot.Status,
		TurnIndex: 0,
		Reused:    reused,
		Reason:    snapshot.Reason,
	}
	if snapshot.Record != nil {
		payload.TurnIndex = snapshot.Record.TurnCount
	}
	return payload
}

func ensureResponseSession(request contract.Request, response contract.Response) contract.Response {
	if response.Session != nil {
		return response
	}
	if request.Session.Mode == "ephemeral" {
		response.Session = &contract.SessionPayload{
			SessionID: "",
			TurnIndex: 0,
			Status:    "ephemeral",
			Reused:    false,
		}
		return response
	}
	if request.Session.SessionID != nil {
		response.Session = &contract.SessionPayload{
			SessionID: *request.Session.SessionID,
			TurnIndex: 0,
			Status:    "unknown",
			Reused:    false,
		}
	}
	return response
}

func sessionWasReused(snapshot session.Snapshot) bool {
	return snapshot.Status == session.ActiveStatus && snapshot.Record != nil
}

func storeCompatibility(record *session.Record, request contract.Request) *string {
	if record == nil {
		return nil
	}
	requestRoot := request.Session.ContextHint.WorkspaceRoot
	if record.WorkspaceRoot != nil && requestRoot != nil && *record.WorkspaceRoot != *requestRoot {
		message := "session workspace mismatch: expected " + *record.WorkspaceRoot + " got " + *requestRoot
		return &message
	}
	return nil
}

func reason(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func modelName(value transport.ModelAware) *string {
	if value == nil {
		return nil
	}
	return value.ModelName()
}

func stringPtr(value string) *string {
	return &value
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
