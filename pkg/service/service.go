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
	Client  transport.Transport
	Store   *session.Store
	OnChunk func([]byte)
}

func (service Service) Run(request contract.Request) (contract.Response, int, error) {
	modelAware, _ := service.Client.(transport.ModelAware)
	if modelAware != nil && !modelAware.HasAuth() {
		response := contract.BuildErrorResponse("invalid_request", "missing auth cookie in ~/.mira/config.json", stringPtr(request.Role), request.RequestID, modelAware.ModelName())
		if request.Session.SessionID != nil {
			response.Session = sessionPayload(*request.Session.SessionID, session.Snapshot{Status: "error"}, false)
		}
		return ensureResponseSession(request, response), runner.ExitInvalidRequest, nil
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
	request, err = attachSessionHistory(request, service.Store)
	if err != nil {
		return contract.Response{}, 0, err
	}
	request, filesRead, err = hydrateRequest(request)
	if err != nil {
		return contract.Response{}, 0, err
	}
	ctx := context.Background()
	cancel := func() {}
	if request.TimeoutSec > 0 {
		ctx, cancel = context.WithTimeout(ctx, time.Duration(request.TimeoutSec)*time.Second)
	}
	defer cancel()
	response, exitCode := runner.ExecuteRequest(ctx, request, service.Client, service.OnChunk)
	response.FilesRead = filesRead
	if modelAware != nil {
		response.Model = modelAware.ModelName()
	}
	if request.Session.Mode == "sticky" && request.Session.SessionID != nil && service.Store != nil {
		response, exitCode, err = finalizeStickyResponse(service, request, snapshot, response, exitCode)
		if err != nil {
			return contract.Response{}, 0, err
		}
		if err := appendJournal(service.Store, request, response); err != nil {
			return contract.Response{}, 0, err
		}
	}
	return ensureResponseSession(request, response), exitCode, nil
}
