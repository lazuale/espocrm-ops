package operation

type RecoveryInfo struct {
	SourceOperationID string
	RequestedMode     string
	AppliedMode       string
	Decision          string
	ResumeStep        string
}

func (r RecoveryInfo) Active() bool {
	return r.SourceOperationID != ""
}
