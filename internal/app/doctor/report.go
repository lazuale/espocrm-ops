package doctor

const (
	statusOK   = "ok"
	statusWarn = "warn"
	statusFail = "fail"
)

func (r Report) Ready() bool {
	for _, check := range r.Checks {
		if check.Status == statusFail {
			return false
		}
	}

	return true
}

func (r Report) Counts() (passed, warnings, failed int) {
	for _, check := range r.Checks {
		switch check.Status {
		case statusOK:
			passed++
		case statusWarn:
			warnings++
		case statusFail:
			failed++
		}
	}

	return passed, warnings, failed
}

func (r *Report) ok(scope, code, summary, details string) {
	r.Checks = append(r.Checks, Check{
		Scope:   scope,
		Code:    code,
		Status:  statusOK,
		Summary: summary,
		Details: details,
	})
}

func (r *Report) warn(scope, code, summary, details, action string) {
	r.Checks = append(r.Checks, Check{
		Scope:   scope,
		Code:    code,
		Status:  statusWarn,
		Summary: summary,
		Details: details,
		Action:  action,
	})
}

func (r *Report) fail(scope, code, summary, details, action string) {
	r.Checks = append(r.Checks, Check{
		Scope:   scope,
		Code:    code,
		Status:  statusFail,
		Summary: summary,
		Details: details,
		Action:  action,
	})
}
