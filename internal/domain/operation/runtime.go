package operation

import "time"

const TimeFormat = time.RFC3339

type Runtime interface {
	Now() time.Time
	NewOperationID() string
}

type DefaultRuntime struct{}

func (DefaultRuntime) Now() time.Time {
	return time.Now().UTC()
}

func (DefaultRuntime) NewOperationID() string {
	return NewOperationID()
}
