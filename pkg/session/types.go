package session

type Snapshot struct {
	Status string
	Record *Record
	Reason *string
}

type Record struct {
	SessionID       string  `json:"session_id"`
	ParentSessionID *string `json:"parent_session_id"`
	CreatedAt       string  `json:"created_at"`
	LastActiveAt    string  `json:"last_active_at"`
	ExpiresAt       string  `json:"expires_at"`
	RemoteSessionID string  `json:"remote_session_id"`
	TurnCount       int     `json:"turn_count"`
	Role            string  `json:"role"`
	ContentFormat   string  `json:"content_format"`
	WorkspaceRoot   *string `json:"workspace_root"`
	TaskDescription *string `json:"task_description"`
	GitBranch       *string `json:"git_branch"`
	TTLSeconds      int     `json:"ttl_seconds"`
	Status          string  `json:"status"`
	LastError       *string `json:"last_error"`
	UpdatedAt       *string `json:"updated_at,omitempty"`
	ExpiredAt       *string `json:"expired_at,omitempty"`
	InvalidatedAt   *string `json:"invalidated_at,omitempty"`
}
