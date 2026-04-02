package session

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

func (store *Store) sessionName(sessionID string) string {
	value := strings.TrimSpace(sessionID)
	if value == "" {
		panic("session_id must be a non-empty string")
	}
	return url.PathEscape(value)
}

func (store *Store) recordPath(sessionID string) string {
	return filepath.Join(store.root, store.sessionName(sessionID)+".json")
}

func (store *Store) lockPath(sessionID string) string {
	return filepath.Join(store.root, store.sessionName(sessionID)+".lock")
}

func requireSessionID(sessionID string) error {
	if strings.TrimSpace(sessionID) == "" {
		return fmt.Errorf("session_id must be a non-empty string")
	}
	return nil
}
