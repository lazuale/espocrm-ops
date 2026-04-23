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
	Running            []string
	DBDump             []byte
	FilesArchive       []byte
	HelperFilesArchive []byte
	InspectErr         error
	StopErr            error
	StartErr           error
	DumpErr            error
	ArchiveErr         error
	HelperArchiveErr   error
	RestoreDBErr       error
	PermissionErr      error
	PostCheckErr       error
	RestoredDBPath     string
	PostCheckServices  []string
	Calls              []string
}

func (r *Static) RunningServices(ctx context.Context, target model.RuntimeTarget) ([]string, error) {
	r.Calls = append(r.Calls, "running_services")
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r.InspectErr != nil {
		return nil, r.InspectErr
	}
	if r.Running != nil {
		return append([]string(nil), r.Running...), nil
	}
	if !r.AppServicesRunning {
		return []string{"db"}, nil
	}
	return []string{"db", "espocrm", "espocrm-daemon", "espocrm-websocket"}, nil
}

func (r *Static) StopServices(ctx context.Context, target model.RuntimeTarget, services ...string) error {
	r.Calls = append(r.Calls, "stop_services")
	if err := ctx.Err(); err != nil {
		return err
	}
	if r.StopErr != nil {
		return r.StopErr
	}
	r.AppServicesRunning = false
	return nil
}

func (r *Static) StartServices(ctx context.Context, target model.RuntimeTarget, services ...string) error {
	r.Calls = append(r.Calls, "start_services")
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

func (r *Static) ArchiveFilesWithHelper(ctx context.Context, target model.RuntimeTarget, contract model.HelperArchiveContract) (io.ReadCloser, error) {
	r.Calls = append(r.Calls, "archive_files_helper")
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r.HelperArchiveErr != nil {
		return nil, r.HelperArchiveErr
	}
	if r.HelperFilesArchive != nil {
		return io.NopCloser(bytes.NewReader(r.HelperFilesArchive)), nil
	}
	return io.NopCloser(bytes.NewReader(r.FilesArchive)), nil
}

func (r *Static) RestoreDatabase(ctx context.Context, target model.RuntimeTarget, dbBackupPath string) error {
	r.Calls = append(r.Calls, "restore_database")
	if err := ctx.Err(); err != nil {
		return err
	}
	if r.RestoreDBErr != nil {
		return r.RestoreDBErr
	}
	r.RestoredDBPath = dbBackupPath
	return nil
}

func (r *Static) ReconcileFilesPermissions(ctx context.Context, target model.RuntimeTarget) error {
	r.Calls = append(r.Calls, "reconcile_files_permissions")
	if err := ctx.Err(); err != nil {
		return err
	}
	return r.PermissionErr
}

func (r *Static) PostRestoreCheck(ctx context.Context, target model.RuntimeTarget, services ...string) error {
	r.Calls = append(r.Calls, "post_restore_check")
	if err := ctx.Err(); err != nil {
		return err
	}
	r.PostCheckServices = append([]string(nil), services...)
	return r.PostCheckErr
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
