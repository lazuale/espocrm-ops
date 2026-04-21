package workflow

type Status string

const (
	StatusWouldRun  Status = "would_run"
	StatusCompleted Status = "completed"
	StatusSkipped   Status = "skipped"
	StatusBlocked   Status = "blocked"
	StatusFailed    Status = "failed"
	StatusNotRun    Status = "not_run"
)

type Step struct {
	Code    string `json:"code"`
	Status  string `json:"status"`
	Summary string `json:"summary"`
	Details string `json:"details,omitempty"`
	Action  string `json:"action,omitempty"`
}

func NewStep(code string, status Status, summary, details, action string) Step {
	return Step{
		Code:    code,
		Status:  string(status),
		Summary: summary,
		Details: details,
		Action:  action,
	}
}
