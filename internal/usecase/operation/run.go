package operation

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/lazuale/espocrm-ops/internal/contract/result"
	domainjournal "github.com/lazuale/espocrm-ops/internal/domain/journal"
	domainoperation "github.com/lazuale/espocrm-ops/internal/domain/operation"
)

type Runtime interface {
	Now() time.Time
	NewOperationID() string
}

type DefaultRuntime struct{}

func (DefaultRuntime) Now() time.Time {
	return domainoperation.DefaultRuntime{}.Now()
}

func (DefaultRuntime) NewOperationID() string {
	return domainoperation.DefaultRuntime{}.NewOperationID()
}

type Writer interface {
	Write(entry domainjournal.Entry) error
}

var ErrJournalWriterDisabled = errors.New("journal writer is not configured")

type DisabledWriter struct{}

func (DisabledWriter) Write(entry domainjournal.Entry) error {
	return ErrJournalWriterDisabled
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
		runtime = domainoperation.DefaultRuntime{}
	}

	return Execution{
		runtime:     runtime,
		writer:      writer,
		command:     command,
		operationID: runtime.NewOperationID(),
		startedAt:   runtime.Now(),
	}
}

func (e Execution) FinishSuccess(res result.Result) (result.Result, error) {
	finishedAt := e.now()
	artifacts, details := serializeJournalPayload(&res)

	res.Command = e.command
	res.OK = true
	res.Timing = &result.TimingInfo{
		StartedAt:  e.startedAt.Format(domainoperation.TimeFormat),
		FinishedAt: finishedAt.Format(domainoperation.TimeFormat),
		DurationMS: finishedAt.Sub(e.startedAt).Milliseconds(),
	}

	if err := e.write(domainjournal.Entry{
		OperationID: e.operationID,
		Command:     e.command,
		StartedAt:   e.startedAt.Format(domainoperation.TimeFormat),
		FinishedAt:  finishedAt.Format(domainoperation.TimeFormat),
		OK:          true,
		DryRun:      res.DryRun,
		Artifacts:   artifacts,
		Details:     details,
		Warnings:    res.Warnings,
	}); err != nil {
		res.Warnings = append(res.Warnings, fmt.Sprintf("failed to write journal entry: %v", err))
	}

	return res, nil
}

func (e Execution) FinishFailure(res result.Result, err error, errCode string) error {
	finishedAt := e.now()
	artifacts, details := serializeJournalPayload(&res)

	return e.write(domainjournal.Entry{
		OperationID:  e.operationID,
		Command:      e.command,
		StartedAt:    e.startedAt.Format(domainoperation.TimeFormat),
		FinishedAt:   finishedAt.Format(domainoperation.TimeFormat),
		OK:           false,
		DryRun:       res.DryRun,
		ErrorCode:    errCode,
		ErrorMessage: err.Error(),
		Artifacts:    artifacts,
		Details:      details,
		Warnings:     res.Warnings,
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

func serializeJournalPayload(res *result.Result) (map[string]any, map[string]any) {
	artifacts, artifactErr := toJSONObjectMap(res.Artifacts)
	if artifactErr != nil {
		res.Warnings = append(res.Warnings, fmt.Sprintf("failed to serialize journal artifacts: %v", artifactErr))
	}

	details, detailsErr := toJSONObjectMap(res.Details)
	if detailsErr != nil {
		res.Warnings = append(res.Warnings, fmt.Sprintf("failed to serialize journal details: %v", detailsErr))
	}

	return artifacts, details
}

func toJSONObjectMap(v any) (map[string]any, error) {
	if v == nil {
		return nil, nil
	}

	raw, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal object: %w", err)
	}

	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("unmarshal object into map: %w", err)
	}

	return out, nil
}
