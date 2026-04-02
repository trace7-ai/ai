package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

type Store struct {
	root string
}

func New(root string) (*Store, error) {
	baseRoot := root
	if baseRoot == "" {
		if envRoot := os.Getenv("MIRA_HOME"); envRoot != "" {
			baseRoot = envRoot
		}
	}
	if baseRoot == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		baseRoot = filepath.Join(home, ".mira")
	}
	sessionsRoot := filepath.Join(baseRoot, "sessions")
	if err := os.MkdirAll(sessionsRoot, 0o755); err != nil {
		return nil, err
	}
	return &Store{root: sessionsRoot}, nil
}

func (store *Store) Lock(sessionID string) (func() error, error) {
	path := store.lockPath(sessionID)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		_ = file.Close()
		return nil, err
	}
	return func() error {
		defer file.Close()
		return syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
	}, nil
}

func (store *Store) Load(sessionID string) (*Record, error) {
	path := store.recordPath(sessionID)
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var record Record
	if err := json.Unmarshal(body, &record); err != nil {
		return nil, fmt.Errorf("session record must be a JSON object: %s", path)
	}
	return &record, nil
}

func (store *Store) Save(sessionID string, record Record) error {
	body, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	path := store.recordPath(sessionID)
	tempFile, err := os.CreateTemp(store.root, "."+store.sessionName(sessionID)+".*.tmp")
	if err != nil {
		return err
	}
	tempPath := tempFile.Name()
	defer os.Remove(tempPath)
	if _, err := tempFile.Write(body); err != nil {
		_ = tempFile.Close()
		return err
	}
	if err := tempFile.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}
