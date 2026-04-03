package sdk

import (
	"fmt"

	"mira/pkg/contract"
	"mira/pkg/roles"
)

type RequestBuilder struct {
	task            string
	requestID       *string
	contentFormat   string
	sessionMode     string
	sessionTouched  bool
	sessionID       *string
	sessionConflict bool
	workspaceRoot   *string
	gitBranch       *string
	diff            string
	files           []contract.ContextFile
	docs            []contract.ContextDoc
	constraints     []string
	promptOverrides *contract.PromptOverrides
	maxTokens       int
	maxTokensSet    bool
	timeoutSec      int
}

func NewRequest(task string) *RequestBuilder {
	return &RequestBuilder{
		task:          task,
		contentFormat: "auto",
		sessionMode:   "ephemeral",
		files:         []contract.ContextFile{},
		docs:          []contract.ContextDoc{},
		constraints:   []string{},
		maxTokens:     contract.DefaultMaxTokens,
		timeoutSec:    contract.DefaultTimeoutSec,
	}
}

func (builder *RequestBuilder) WithSession(sessionID string) *RequestBuilder {
	builder.setSessionMode("sticky")
	builder.sessionID = cloneString(sessionID)
	return builder
}

func (builder *RequestBuilder) WithEphemeral() *RequestBuilder {
	builder.setSessionMode("ephemeral")
	builder.sessionID = nil
	return builder
}

func (builder *RequestBuilder) WithContentFormat(format string) *RequestBuilder {
	builder.contentFormat = format
	return builder
}

func (builder *RequestBuilder) WithTimeoutSec(seconds int) *RequestBuilder {
	builder.timeoutSec = seconds
	return builder
}

func (builder *RequestBuilder) WithMaxTokens(tokens int) *RequestBuilder {
	builder.maxTokens = tokens
	builder.maxTokensSet = true
	return builder
}

func (builder *RequestBuilder) WithWorkspaceRoot(path string) *RequestBuilder {
	builder.workspaceRoot = cloneString(path)
	return builder
}

func (builder *RequestBuilder) WithGitBranch(branch string) *RequestBuilder {
	builder.gitBranch = cloneString(branch)
	return builder
}

func (builder *RequestBuilder) WithDiff(diff string) *RequestBuilder {
	builder.diff = diff
	return builder
}

func (builder *RequestBuilder) AddFile(path, content string) *RequestBuilder {
	return builder.AddContextFile(contract.ContextFile{Path: path, Content: content})
}

func (builder *RequestBuilder) AddContextFile(file contract.ContextFile) *RequestBuilder {
	builder.files = append(builder.files, file)
	return builder
}

func (builder *RequestBuilder) AddDoc(content string) *RequestBuilder {
	return builder.AddContextDoc(contract.ContextDoc{Content: content})
}

func (builder *RequestBuilder) AddContextDoc(doc contract.ContextDoc) *RequestBuilder {
	builder.docs = append(builder.docs, doc)
	return builder
}

func (builder *RequestBuilder) AddConstraint(text string) *RequestBuilder {
	builder.constraints = append(builder.constraints, text)
	return builder
}

func (builder *RequestBuilder) WithPromptProtocol(text string) *RequestBuilder {
	builder.ensurePromptOverrides().Protocol = cloneString(text)
	return builder
}

func (builder *RequestBuilder) WithOutputInstructions(text string) *RequestBuilder {
	builder.ensurePromptOverrides().OutputInstructions = cloneString(text)
	return builder
}

func (builder *RequestBuilder) WithRequestID(id string) *RequestBuilder {
	builder.requestID = cloneString(id)
	return builder
}

func (builder *RequestBuilder) Build() (contract.Request, error) {
	if builder.sessionConflict {
		return contract.Request{}, fmt.Errorf("cannot combine sticky and ephemeral session modes in one request builder")
	}
	if builder.maxTokensSet && builder.maxTokens == 0 {
		return contract.Request{}, fmt.Errorf("max_tokens must be an integer between 256 and %d", contract.MaxMaxTokens)
	}
	return contract.ValidateRequest(builder.request())
}

func (builder *RequestBuilder) request() contract.Request {
	return contract.Request{
		Version:       contract.SchemaVersion,
		Role:          roles.Assistant,
		RequestID:     cloneStringPtr(builder.requestID),
		ContentFormat: builder.contentFormat,
		Session: contract.Session{
			Mode:      builder.sessionMode,
			SessionID: cloneStringPtr(builder.sessionID),
			ContextHint: contract.SessionContextHint{
				WorkspaceRoot: cloneStringPtr(builder.workspaceRoot),
				GitBranch:     cloneStringPtr(builder.gitBranch),
			},
		},
		FileManifest: contract.FileManifest{
			Mode:          "none",
			Paths:         []string{},
			MaxTotalBytes: contract.DefaultMaxFileSize,
			ReadOnly:      true,
		},
		Task:        builder.task,
		Constraints: cloneStrings(builder.constraints),
		Context: contract.Context{
			Diff:  builder.diff,
			Files: cloneFiles(builder.files),
			Docs:  cloneDocs(builder.docs),
		},
		PromptOverrides: clonePromptOverrides(builder.promptOverrides),
		MaxTokens:       builder.maxTokens,
		TimeoutSec:      builder.timeoutSec,
	}
}

func (builder *RequestBuilder) setSessionMode(mode string) {
	if builder.sessionTouched && builder.sessionMode != mode {
		builder.sessionConflict = true
	}
	builder.sessionTouched = true
	builder.sessionMode = mode
}

func (builder *RequestBuilder) ensurePromptOverrides() *contract.PromptOverrides {
	if builder.promptOverrides == nil {
		builder.promptOverrides = &contract.PromptOverrides{}
	}
	return builder.promptOverrides
}
