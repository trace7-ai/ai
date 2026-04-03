package job

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"time"

	"mira/pkg/contract"
	"mira/pkg/service"
	"mira/pkg/session"
	"mira/pkg/transport"
)

type Record struct {
	JobID        string  `json:"job_id"`
	SessionID    *string `json:"session_id,omitempty"`
	Status       string  `json:"status"`
	SubmittedAt  string  `json:"submitted_at"`
	StartedAt    *string `json:"started_at,omitempty"`
	FinishedAt   *string `json:"finished_at,omitempty"`
	WorkerPID    *int    `json:"worker_pid,omitempty"`
	RequestPath  string  `json:"request_path"`
	ResponsePath string  `json:"response_path"`
	ExitCode     *int    `json:"exit_code,omitempty"`
	Error        *string `json:"error,omitempty"`
}

const (
	QueuedStatus    = "queued"
	RunningStatus   = "running"
	CompletedStatus = "completed"
	FailedStatus    = "failed"
)

type Store struct {
	root string
}

type TransportFactory func() (transport.Transport, error)

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
	jobsRoot := filepath.Join(baseRoot, "jobs")
	if err := os.MkdirAll(jobsRoot, 0o755); err != nil {
		return nil, err
	}
	return &Store{root: jobsRoot}, nil
}

func (store *Store) Create(request contract.Request) (Record, error) {
	jobID := newJobID()
	record := Record{
		JobID:        jobID,
		SessionID:    request.Session.SessionID,
		Status:       QueuedStatus,
		SubmittedAt:  nowISO(),
		RequestPath:  store.requestPath(jobID),
		ResponsePath: store.responsePath(jobID),
	}
	if err := writeJSONFile(record.RequestPath, request); err != nil {
		return Record{}, err
	}
	if err := store.Save(record); err != nil {
		return Record{}, err
	}
	return record, nil
}

func (store *Store) Load(jobID string) (*Record, error) {
	body, err := os.ReadFile(store.recordPath(jobID))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var record Record
	if err := json.Unmarshal(body, &record); err != nil {
		return nil, fmt.Errorf("job record must be a JSON object: %s", store.recordPath(jobID))
	}
	return &record, nil
}

func (store *Store) Save(record Record) error {
	return writeJSONFile(store.recordPath(record.JobID), record)
}

func (store *Store) LoadRequest(jobID string) (contract.Request, error) {
	var request contract.Request
	if err := readJSONFile(store.requestPath(jobID), &request); err != nil {
		return contract.Request{}, err
	}
	return request, nil
}

func (store *Store) SaveResponse(jobID string, response contract.Response) error {
	return writeJSONFile(store.responsePath(jobID), response)
}

func (store *Store) LoadResponse(jobID string) (*contract.Response, error) {
	path := store.responsePath(jobID)
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var response contract.Response
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("job response must be a JSON object: %s", path)
	}
	return &response, nil
}

func (store *Store) List(limit int) ([]Record, error) {
	paths, err := filepath.Glob(filepath.Join(store.root, "*.json"))
	if err != nil {
		return nil, err
	}
	records := make([]Record, 0, len(paths))
	for _, path := range paths {
		if filepath.Ext(path) != ".json" || isDataFile(path) {
			continue
		}
		var record Record
		if err := readJSONFile(path, &record); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool {
		return records[i].SubmittedAt > records[j].SubmittedAt
	})
	if limit > 0 && len(records) > limit {
		return records[:limit], nil
	}
	return records, nil
}

func (store *Store) recordPath(jobID string) string { return filepath.Join(store.root, jobID+".json") }
func (store *Store) requestPath(jobID string) string {
	return filepath.Join(store.root, jobID+".request.json")
}
func (store *Store) responsePath(jobID string) string {
	return filepath.Join(store.root, jobID+".response.json")
}

func writeJSONFile(path string, value any) error {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	tempFile, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
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

func readJSONFile(path string, target any) error {
	body, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, target)
}

func newJobID() string {
	return fmt.Sprintf("job-%s-%06d", time.Now().UTC().Format("20060102T150405.000000000"), rand.Intn(1000000))
}

func nowISO() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05+00:00")
}

func isDataFile(path string) bool {
	base := filepath.Base(path)
	return len(base) > 13 && (base[len(base)-13:] == ".request.json" || base[len(base)-14:] == ".response.json")
}

func (record Record) Start(pid int, startedAt string) Record {
	updated := record
	updated.Status = RunningStatus
	updated.StartedAt = &startedAt
	updated.WorkerPID = &pid
	updated.Error = nil
	return updated
}

func (record Record) Complete(exitCode int, finishedAt string) Record {
	updated := record
	updated.Status = CompletedStatus
	updated.FinishedAt = &finishedAt
	updated.ExitCode = &exitCode
	updated.Error = nil
	return updated
}

func (record Record) Fail(message string, exitCode int, finishedAt string) Record {
	updated := record
	updated.Status = FailedStatus
	updated.FinishedAt = &finishedAt
	updated.ExitCode = &exitCode
	updated.Error = &message
	return updated
}

func IsTerminal(status string) bool {
	return status == CompletedStatus || status == FailedStatus
}

func Execute(jobID string, factory TransportFactory) (exitCode int, err error) {
	store, err := New("")
	if err != nil {
		return 2, err
	}
	record, err := store.Load(jobID)
	if err != nil {
		return 2, err
	}
	if record == nil {
		return 2, fmt.Errorf("job not found: %s", jobID)
	}
	defer recoverWorker(store, *record, &exitCode, &err)

	running := record.Start(os.Getpid(), nowISO())
	if err := store.Save(running); err != nil {
		return 2, err
	}
	request, err := store.LoadRequest(jobID)
	if err != nil {
		return fail(store, running, 2, err), nil
	}
	client, err := factory()
	if err != nil {
		return fail(store, running, 2, err), nil
	}
	sessionStore, err := session.New("")
	if err != nil {
		return fail(store, running, 2, err), nil
	}
	response, code, runErr := service.Service{Client: client, Store: sessionStore}.Run(request)
	if runErr != nil {
		return fail(store, running, codeOrDefault(code, 1), runErr), nil
	}
	if err := store.SaveResponse(jobID, response); err != nil {
		return fail(store, running, 1, err), nil
	}
	completed := running.Complete(code, nowISO())
	if err := store.Save(completed); err != nil {
		return 1, err
	}
	return code, nil
}

func recoverWorker(store *Store, record Record, exitCode *int, err *error) {
	value := recover()
	if value == nil {
		return
	}
	message := fmt.Sprintf("panic: %v", value)
	*exitCode = fail(store, record, 1, fmt.Errorf("%s", message))
	*err = nil
}

func fail(store *Store, record Record, exitCode int, runErr error) int {
	failed := record.Fail(runErr.Error(), exitCode, nowISO())
	_ = store.Save(failed)
	return exitCode
}

func codeOrDefault(code int, fallback int) int {
	if code == 0 {
		return fallback
	}
	return code
}
