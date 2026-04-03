package service

import (
	"mira/pkg/contract"
	"mira/pkg/session"
)

func attachSessionHistory(request contract.Request, store *session.Store) (contract.Request, error) {
	if store == nil || request.Session.Mode != "sticky" || request.Session.SessionID == nil {
		return request, nil
	}
	entries, err := store.ReadJournal(*request.Session.SessionID, session.PromptHistoryLimit)
	if err != nil {
		return contract.Request{}, err
	}
	doc := session.JournalContextDoc(entries)
	if doc == nil {
		return request, nil
	}
	request.Context.Docs = append(request.Context.Docs, *doc)
	return request, nil
}

func appendJournal(store *session.Store, request contract.Request, response contract.Response) error {
	if store == nil || request.Session.Mode != "sticky" || request.Session.SessionID == nil {
		return nil
	}
	if response.Session == nil {
		return nil
	}
	return store.AppendJournalEntry(*request.Session.SessionID, session.BuildJournalEntry(request, response))
}
