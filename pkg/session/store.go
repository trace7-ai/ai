package session

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"mira/pkg/contract"
)

const (
	MissingStatus = "missing"
	ActiveStatus  = "active"
	InvalidStatus = "invalid"
)

type Store struct {
	root    string
	pinRoot string
}

type Snapshot struct {
	Status string
	Record *Record
	Reason *string
}

type TaskPin struct {
	WorkspaceRoot   string  `json:"workspace_root"`
	SessionID       string  `json:"session_id"`
	TaskDescription *string `json:"task_description,omitempty"`
	UpdatedAt       string  `json:"updated_at"`
}

type Record struct {
	SessionID       string  `json:"session_id"`
	ParentSessionID *string `json:"parent_session_id"`
	CreatedAt       string  `json:"created_at"`
	LastActiveAt    string  `json:"last_active_at"`
	RemoteSessionID string  `json:"remote_session_id"`
	TurnCount       int     `json:"turn_count"`
	Role            string  `json:"role"`
	ContentFormat   string  `json:"content_format"`
	WorkspaceRoot   *string `json:"workspace_root"`
	TaskDescription *string `json:"task_description"`
	GitBranch       *string `json:"git_branch"`
	Status          string  `json:"status"`
	LastError       *string `json:"last_error"`
	UpdatedAt       *string `json:"updated_at,omitempty"`
	InvalidatedAt   *string `json:"invalidated_at,omitempty"`
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
	pinsRoot := filepath.Join(baseRoot, "session_pins")
	if err := os.MkdirAll(pinsRoot, 0o755); err != nil {
		return nil, err
	}
	return &Store{root: sessionsRoot, pinRoot: pinsRoot}, nil
}

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

func (store *Store) journalPath(sessionID string) string {
	return filepath.Join(store.root, store.sessionName(sessionID)+".journal.jsonl")
}

func (store *Store) pinPath(workspaceRoot string) string {
	return filepath.Join(store.pinRoot, store.sessionName(workspaceRoot)+".json")
}

func (store *Store) pinLockPath(workspaceRoot string) string {
	return filepath.Join(store.pinRoot, store.sessionName(workspaceRoot)+".lock")
}

func requireSessionID(sessionID string) error {
	if strings.TrimSpace(sessionID) == "" {
		return fmt.Errorf("session_id must be a non-empty string")
	}
	return nil
}

func (store *Store) Lock(sessionID string) (func() error, error) {
	return store.lockFile(store.lockPath(sessionID))
}

func (store *Store) LockTaskPin(workspaceRoot string) (func() error, error) {
	if strings.TrimSpace(workspaceRoot) == "" {
		return nil, fmt.Errorf("workspace_root must be a non-empty string")
	}
	return store.lockFile(store.pinLockPath(workspaceRoot))
}

func (store *Store) lockFile(path string) (func() error, error) {
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
	return store.writeJSONFile(store.root, "."+store.sessionName(sessionID)+".*.tmp", store.recordPath(sessionID), record)
}

func (store *Store) LoadTaskPin(workspaceRoot string) (*TaskPin, error) {
	if strings.TrimSpace(workspaceRoot) == "" {
		return nil, fmt.Errorf("workspace_root must be a non-empty string")
	}
	body, err := os.ReadFile(store.pinPath(workspaceRoot))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var pin TaskPin
	if err := json.Unmarshal(body, &pin); err != nil {
		return nil, fmt.Errorf("task pin must be a JSON object: %s", store.pinPath(workspaceRoot))
	}
	if strings.TrimSpace(pin.SessionID) == "" {
		return nil, fmt.Errorf("task pin is missing session_id: %s", store.pinPath(workspaceRoot))
	}
	return &pin, nil
}

func (store *Store) SaveTaskPin(workspaceRoot, sessionID string, taskDescription *string) error {
	if strings.TrimSpace(workspaceRoot) == "" {
		return fmt.Errorf("workspace_root must be a non-empty string")
	}
	if err := requireSessionID(sessionID); err != nil {
		return err
	}
	pin := TaskPin{
		WorkspaceRoot:   workspaceRoot,
		SessionID:       sessionID,
		TaskDescription: taskDescription,
		UpdatedAt:       toISOFormat(nowUTC()),
	}
	return store.writeJSONFile(store.pinRoot, "."+store.sessionName(workspaceRoot)+".*.tmp", store.pinPath(workspaceRoot), pin)
}

func (store *Store) writeJSONFile(root, pattern, path string, value any) error {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	tempFile, err := os.CreateTemp(root, pattern)
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

func BuildRecord(request contract.Request, remoteSessionID string, existing *Record) (Record, error) {
	if remoteSessionID == "" {
		return Record{}, fmt.Errorf("remote_session_id must be a non-empty string")
	}
	turnCount := 1
	createdAt := toISOFormat(nowUTC())
	if existing != nil {
		turnCount = existing.TurnCount + 1
		createdAt = existing.CreatedAt
	}
	return Record{
		SessionID:       derefString(request.Session.SessionID),
		ParentSessionID: request.Session.ParentSessionID,
		CreatedAt:       createdAt,
		LastActiveAt:    toISOFormat(nowUTC()),
		RemoteSessionID: remoteSessionID,
		TurnCount:       turnCount,
		Role:            request.Role,
		ContentFormat:   request.ContentFormat,
		WorkspaceRoot:   request.Session.ContextHint.WorkspaceRoot,
		TaskDescription: request.Session.ContextHint.TaskDescription,
		GitBranch:       request.Session.ContextHint.GitBranch,
		Status:          ActiveStatus,
		LastError:       nil,
	}, nil
}

func (store *Store) CompatibilityError(record *Record, request contract.Request) *string {
	if record == nil {
		return nil
	}
	requestRoot := request.Session.ContextHint.WorkspaceRoot
	if record.WorkspaceRoot != nil && requestRoot != nil && *record.WorkspaceRoot != *requestRoot {
		message := fmt.Sprintf("session workspace mismatch: expected %s got %s", *record.WorkspaceRoot, *requestRoot)
		return &message
	}
	return nil
}

func (store *Store) MarkInvalid(sessionID string, record Record, reason string) (Record, error) {
	invalid := withStatus(record, InvalidStatus, reason)
	return invalid, store.Save(sessionID, invalid)
}

func toISOFormat(value time.Time) string {
	return value.UTC().Format("2006-01-02T15:04:05+00:00")
}

func withStatus(record Record, status string, reason string) Record {
	updated := record
	now := toISOFormat(time.Now().UTC())
	updated.Status = status
	updated.LastError = &reason
	updated.UpdatedAt = &now
	if status == InvalidStatus {
		updated.InvalidatedAt = &now
	}
	return updated
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func nowUTC() time.Time {
	return time.Now().UTC()
}
