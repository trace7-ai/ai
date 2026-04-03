package cli

type ParsedArgs struct {
	InputFile     string
	Format        string
	ContentFormat string
	Ephemeral     bool
	NewSession    bool
	Role          string
	Task          string
	Session       string
	ContextFile   string
	Files         []string
	WorkspaceRoot string
	TaskDesc      string
	GitBranch     string
	TimeoutSec    int
	MaxTokens     int
	RequestID     string
	TaskParts     []string
	InlineTask    string
}
