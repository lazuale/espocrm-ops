package operation

import (
	"errors"
	"fmt"
	"time"

	domainjournal "github.com/lazuale/espocrm-ops/internal/domain/journal"
)

type Runtime interface {
	Now() time.Time
	NewOperationID() string
}

type Writer interface {
	Write(entry domainjournal.Entry) error
}

var ErrJournalWriterDisabled = errors.New("journal writer is not configured")

type DisabledWriter struct{}

func (DisabledWriter) Write(entry domainjournal.Entry) error {
	return ErrJournalWriterDisabled
}

type JournalPayload struct {
	Artifacts map[string]any
	Details   map[string]any
	Items     []any
}

type JournalRecord struct {
	DryRun   bool
	Message  string
	Warnings []string
	Payload  JournalPayload
}

type Completion struct {
	StartedAt  string
	FinishedAt string
	DurationMS int64
	Warnings   []string
}

type Execution struct {
	runtime     Runtime
	writer      Writer
	command     string
	operationID string
	startedAt   time.Time
}

func Begin(runtime Runtime, writer Writer, command string) Execution {
	if runtime == nil {
		runtime = DefaultRuntime{}
	}

	return Execution{
		runtime:     runtime,
		writer:      writer,
		command:     command,
		operationID: runtime.NewOperationID(),
		startedAt:   runtime.Now(),
	}
}

func (e Execution) FinishSuccess(record JournalRecord) (Completion, error) {
	finishedAt := e.now()
	completion := Completion{
		StartedAt:  e.startedAt.Format(TimeFormat),
		FinishedAt: finishedAt.Format(TimeFormat),
		DurationMS: finishedAt.Sub(e.startedAt).Milliseconds(),
		Warnings:   append([]string(nil), record.Warnings...),
	}

	if err := e.write(domainjournal.Entry{
		OperationID: e.operationID,
		Command:     e.command,
		StartedAt:   completion.StartedAt,
		FinishedAt:  completion.FinishedAt,
		OK:          true,
		DryRun:      record.DryRun,
		Message:     record.Message,
		Artifacts:   record.Payload.Artifacts,
		Details:     record.Payload.Details,
		Items:       record.Payload.Items,
		Warnings:    record.Warnings,
	}); err != nil {
		completion.Warnings = append(completion.Warnings, fmt.Sprintf("failed to write journal entry: %v", err))
	}

	return completion, nil
}

func (e Execution) FinishFailure(record JournalRecord, err error, errCode string) error {
	finishedAt := e.now()

	return e.write(domainjournal.Entry{
		OperationID:  e.operationID,
		Command:      e.command,
		StartedAt:    e.startedAt.Format(TimeFormat),
		FinishedAt:   finishedAt.Format(TimeFormat),
		OK:           false,
		DryRun:       record.DryRun,
		Message:      record.Message,
		ErrorCode:    errCode,
		ErrorMessage: err.Error(),
		Artifacts:    record.Payload.Artifacts,
		Details:      record.Payload.Details,
		Items:        record.Payload.Items,
		Warnings:     record.Warnings,
	})
}

func (e Execution) now() time.Time {
	if e.runtime == nil {
		return time.Now().UTC()
	}

	return e.runtime.Now()
}

func (e Execution) write(entry domainjournal.Entry) error {
	if e.writer == nil {
		return ErrJournalWriterDisabled
	}

	return e.writer.Write(entry)
}
