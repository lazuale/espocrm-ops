package workflow

type Status string

const (
	StatusPlanned   Status = "would_run"
	StatusCompleted Status = "completed"
	StatusSkipped   Status = "skipped"
	StatusBlocked   Status = "blocked"
	StatusFailed    Status = "failed"
)

type Step struct {
	Code    string `json:"code"`
	Status  Status `json:"status"`
	Summary string `json:"summary"`
	Details string `json:"details,omitempty"`
	Action  string `json:"action,omitempty"`
}

func NewStep(code string, status Status, summary, details, action string) Step {
	return Step{
		Code:    code,
		Status:  status,
		Summary: summary,
		Details: details,
		Action:  action,
	}
}

func (s Status) String() string {
	return string(s)
}
