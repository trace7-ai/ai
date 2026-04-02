package contract

type SessionContextHint struct {
	WorkspaceRoot   *string `json:"workspace_root"`
	TaskDescription *string `json:"task_description"`
	GitBranch       *string `json:"git_branch"`
}

type Session struct {
	Mode            string             `json:"mode"`
	SessionID       *string            `json:"session_id"`
	ParentSessionID *string            `json:"parent_session_id"`
	ContextHint     SessionContextHint `json:"context_hint"`
}

type FileManifest struct {
	Mode          string   `json:"mode"`
	Paths         []string `json:"paths"`
	MaxTotalBytes int      `json:"max_total_bytes"`
	ReadOnly      bool     `json:"read_only"`
}

type ContextFile struct {
	Path    string  `json:"path"`
	Content string  `json:"content"`
	Source  *string `json:"source,omitempty"`
	Title   *string `json:"title,omitempty"`
}

type ContextDoc struct {
	Content string  `json:"content"`
	Source  *string `json:"source,omitempty"`
	Title   *string `json:"title,omitempty"`
}

type Context struct {
	Diff  string        `json:"diff"`
	Files []ContextFile `json:"files"`
	Docs  []ContextDoc  `json:"docs"`
}

type Request struct {
	Version       string       `json:"version"`
	Role          string       `json:"role"`
	RequestID     *string      `json:"request_id"`
	ContentFormat string       `json:"content_format"`
	Session       Session      `json:"session"`
	FileManifest  FileManifest `json:"file_manifest"`
	Task          string       `json:"task"`
	Constraints   []string     `json:"constraints"`
	Context       Context      `json:"context"`
	MaxTokens     int          `json:"max_tokens"`
	TimeoutSec    int          `json:"timeout_sec"`
}

type ErrorItem struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type TokenUsage struct {
	Input  *int `json:"input"`
	Output *int `json:"output"`
}

type SessionPayload struct {
	SessionID  string  `json:"session_id"`
	TurnIndex  int     `json:"turn_index"`
	Status     string  `json:"status"`
	Reason     *string `json:"reason"`
	TTLSeconds *int    `json:"ttl_seconds"`
	ExpiresAt  *string `json:"expires_at"`
}

type Response struct {
	Version     string           `json:"version"`
	Status      string           `json:"status"`
	Role        *string          `json:"role"`
	RequestID   *string          `json:"request_id"`
	Model       *string          `json:"model"`
	ContentType *string          `json:"content_type"`
	Result      any              `json:"result"`
	Errors      []ErrorItem      `json:"errors"`
	TokenUsage  TokenUsage       `json:"token_usage"`
	Truncated   bool             `json:"truncated"`
	FilesRead   []map[string]any `json:"files_read"`
	Session     *SessionPayload  `json:"session"`
}
