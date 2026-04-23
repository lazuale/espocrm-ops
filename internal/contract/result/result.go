package result

type Result struct {
	Command         string           `json:"command"`
	OK              bool             `json:"ok"`
	ProcessExitCode *int             `json:"process_exit_code,omitempty"`
	Message         string           `json:"message,omitempty"`
	Error           *ErrorInfo       `json:"error,omitempty"`
	Warnings        []string         `json:"warnings,omitempty"`
	Details         DetailsPayload   `json:"details,omitempty"`
	Artifacts       ArtifactsPayload `json:"artifacts,omitempty"`
	Timing          *TimingInfo      `json:"timing,omitempty"`
	DryRun          bool             `json:"dry_run,omitempty"`
	Items           []ItemPayload    `json:"items,omitempty"`
}

type ErrorInfo struct {
	Code     string `json:"code"`
	Kind     string `json:"kind,omitempty"`
	ExitCode int    `json:"exit_code,omitempty"`
	Message  string `json:"message"`
}

type TimingInfo struct {
	StartedAt  string `json:"started_at,omitempty"`
	FinishedAt string `json:"finished_at,omitempty"`
	DurationMS int64  `json:"duration_ms,omitempty"`
}
