package service

import (
	"context"
	"time"

	"mira/pkg/contract"
	"mira/pkg/runner"
	"mira/pkg/session"
	"mira/pkg/transport"
)

type Service struct {
	Client transport.Transport
	Store  *session.Store
}

func (service Service) Run(request contract.Request) (contract.Response, int, error) {
	modelAware, _ := service.Client.(transport.ModelAware)
	if modelAware != nil && !modelAware.HasAuth() {
		return contract.BuildErrorResponse("invalid_request", "missing auth cookie in ~/.mira/config.json", stringPtr(request.Role), request.RequestID, modelAware.ModelName()), runner.ExitInvalidRequest, nil
	}
	var (
		filesRead []map[string]any
		snapshot  session.Snapshot
		err       error
	)
	if request.Session.Mode == "sticky" && request.Session.SessionID != nil && service.Store != nil {
		unlock, lockErr := service.Store.Lock(*request.Session.SessionID)
		if lockErr != nil {
			return contract.Response{}, 0, lockErr
		}
		defer unlock()
		snapshot, err = service.Store.Inspect(*request.Session.SessionID)
		if err != nil {
			return contract.Response{}, 0, err
		}
		if rejected, exitCode := rejectSession(request, snapshot, modelAware); rejected != nil {
			return *rejected, exitCode, nil
		}
		if sessionAware, ok := service.Client.(transport.SessionAware); ok && snapshot.Status == session.ActiveStatus && snapshot.Record != nil {
			sessionAware.SetRemoteSessionID(snapshot.Record.RemoteSessionID)
		}
	}
	request, filesRead, err = hydrateRequest(request)
	if err != nil {
		return contract.Response{}, 0, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(request.TimeoutSec)*time.Second)
	defer cancel()
	response, exitCode := runner.ExecuteRequest(ctx, request, service.Client)
	response.FilesRead = filesRead
	if modelAware != nil {
		response.Model = modelAware.ModelName()
	}
	if request.Session.Mode == "sticky" && request.Session.SessionID != nil && service.Store != nil {
		response, exitCode, err = finalizeStickyResponse(service, request, snapshot, response, exitCode)
		if err != nil {
			return contract.Response{}, 0, err
		}
	}
	return response, exitCode, nil
}
