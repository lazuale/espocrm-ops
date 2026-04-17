package journal

type Entry struct {
	OperationID  string         `json:"operation_id"`
	Command      string         `json:"command"`
	StartedAt    string         `json:"started_at"`
	FinishedAt   string         `json:"finished_at,omitempty"`
	OK           bool           `json:"ok"`
	DryRun       bool           `json:"dry_run,omitempty"`
	ErrorCode    string         `json:"error_code,omitempty"`
	ErrorMessage string         `json:"error_message,omitempty"`
	Artifacts    map[string]any `json:"artifacts,omitempty"`
	Details      map[string]any `json:"details,omitempty"`
	Warnings     []string       `json:"warnings,omitempty"`
}
