package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type SessionSummary struct {
	SessionID       string  `json:"session_id"`
	Status          string  `json:"status"`
	LastActiveAt    string  `json:"last_active_at"`
	TurnCount       int     `json:"turn_count"`
	TaskDescription *string `json:"task_description,omitempty"`
}

func (store *Store) ListSessions(limit int) ([]SessionSummary, error) {
	pattern := filepath.Join(store.root, "*.json")
	paths, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	sessions := make([]SessionSummary, 0, len(paths))
	for _, path := range paths {
		record, err := store.loadRecordPath(path)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, SessionSummary{
			SessionID:       record.SessionID,
			Status:          record.Status,
			LastActiveAt:    record.LastActiveAt,
			TurnCount:       record.TurnCount,
			TaskDescription: record.TaskDescription,
		})
	}
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].LastActiveAt > sessions[j].LastActiveAt
	})
	if limit > 0 && len(sessions) > limit {
		return sessions[:limit], nil
	}
	return sessions, nil
}

func (store *Store) SearchJournal(query string, limit int) ([]JournalEntry, error) {
	pattern := filepath.Join(store.root, "*.journal.jsonl")
	paths, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	matches := []JournalEntry{}
	lowered := strings.ToLower(strings.TrimSpace(query))
	for _, path := range paths {
		file, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		err = visitJournalEntries(file, path, func(entry JournalEntry) error {
			if lowered == "" || historyMatches(entry, lowered) {
				matches = append(matches, entry)
			}
			return nil
		})
		closeErr := file.Close()
		if err != nil {
			return nil, err
		}
		if closeErr != nil {
			return nil, closeErr
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Timestamp > matches[j].Timestamp
	})
	if limit > 0 && len(matches) > limit {
		return matches[:limit], nil
	}
	return matches, nil
}

func (store *Store) loadRecordPath(path string) (Record, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return Record{}, err
	}
	var record Record
	if err := json.Unmarshal(body, &record); err != nil {
		return Record{}, err
	}
	return record, nil
}

func historyMatches(entry JournalEntry, query string) bool {
	fields := []string{entry.SessionID, entry.Task, entry.Summary, entry.Status}
	if entry.CarryForward != nil {
		fields = append(fields, *entry.CarryForward)
	}
	for _, field := range fields {
		if strings.Contains(strings.ToLower(field), query) {
			return true
		}
	}
	return false
}
