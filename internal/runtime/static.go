package runtime

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/lazuale/espocrm-ops/internal/model"
)

type Static struct {
	AppServicesRunning bool
	DBDump             []byte
	FilesArchive       []byte
	InspectErr         error
	StopErr            error
	StartErr           error
	DumpErr            error
	ArchiveErr         error
	Calls              []string
}

func (r *Static) InspectApplication(ctx context.Context, target model.RuntimeTarget) (model.RuntimeState, error) {
	r.Calls = append(r.Calls, "inspect_application")
	if err := ctx.Err(); err != nil {
		return model.RuntimeState{}, err
	}
	if r.InspectErr != nil {
		return model.RuntimeState{}, r.InspectErr
	}
	return model.RuntimeState{AppServicesRunning: r.AppServicesRunning}, nil
}

func (r *Static) StopApplication(ctx context.Context, target model.RuntimeTarget) error {
	r.Calls = append(r.Calls, "stop_application")
	if err := ctx.Err(); err != nil {
		return err
	}
	if r.StopErr != nil {
		return r.StopErr
	}
	r.AppServicesRunning = false
	return nil
}

func (r *Static) StartApplication(ctx context.Context, target model.RuntimeTarget) error {
	r.Calls = append(r.Calls, "start_application")
	if err := ctx.Err(); err != nil {
		return err
	}
	if r.StartErr != nil {
		return r.StartErr
	}
	r.AppServicesRunning = true
	return nil
}

func (r *Static) DumpDatabase(ctx context.Context, target model.RuntimeTarget) (io.ReadCloser, error) {
	r.Calls = append(r.Calls, "dump_database")
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r.DumpErr != nil {
		return nil, r.DumpErr
	}
	return io.NopCloser(bytes.NewReader(r.DBDump)), nil
}

func (r *Static) ArchiveFiles(ctx context.Context, target model.RuntimeTarget) (io.ReadCloser, error) {
	r.Calls = append(r.Calls, "archive_files")
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r.ArchiveErr != nil {
		return nil, r.ArchiveErr
	}
	return io.NopCloser(bytes.NewReader(r.FilesArchive)), nil
}

func (r *Static) RequireCallOrder(want []string) error {
	if len(r.Calls) != len(want) {
		return fmt.Errorf("получены runtime calls %v, ожидались %v", r.Calls, want)
	}
	for i := range want {
		if r.Calls[i] != want[i] {
			return fmt.Errorf("runtime call #%d: получен %q, ожидался %q", i, r.Calls[i], want[i])
		}
	}
	return nil
}
