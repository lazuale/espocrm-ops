package ops

import (
	"context"

	"github.com/lazuale/espocrm-ops/internal/config"
	"github.com/lazuale/espocrm-ops/internal/runtime"
)

type DoctorCheck struct {
	Name string `json:"name"`
	OK   bool   `json:"ok"`
}

type DoctorResult struct {
	Checks []DoctorCheck `json:"checks"`
}

type doctorRuntime interface {
	ComposeConfig(ctx context.Context, target runtime.Target) error
	RequireHealthyServices(ctx context.Context, target runtime.Target, services []string) error
	DBPing(ctx context.Context, target runtime.Target) error
}

func Doctor(ctx context.Context, req config.Request, rt doctorRuntime) (DoctorResult, error) {
	if rt == nil {
		return DoctorResult{}, runtimeError("doctor runtime is required", nil)
	}

	result := DoctorResult{Checks: make([]DoctorCheck, 0, 7)}
	cfg, err := config.Load(req)
	if err != nil {
		return failDoctorCheck(result, "config", ErrorKindUsage, "doctor config check failed", err)
	}
	result.Checks = append(result.Checks, passedDoctorCheck("config"))

	target := targetFromConfig(cfg)
	if err := rt.ComposeConfig(ctx, target); err != nil {
		return failDoctorCheck(result, "compose_config", ErrorKindRuntime, "doctor compose config check failed", err)
	}
	result.Checks = append(result.Checks, passedDoctorCheck("compose_config"))

	if err := ensureBackupRootWritable(cfg.BackupRoot, false); err != nil {
		return failDoctorCheck(result, "backup_root", ErrorKindIO, "doctor backup root check failed", err)
	}
	result.Checks = append(result.Checks, passedDoctorCheck("backup_root"))

	if err := ensureStorageDir(cfg.StorageDir); err != nil {
		return failDoctorCheck(result, "storage_dir", ErrorKindIO, "doctor storage dir check failed", err)
	}
	result.Checks = append(result.Checks, passedDoctorCheck("storage_dir"))

	if err := runtime.TarExists(); err != nil {
		return failDoctorCheck(result, "tar", ErrorKindRuntime, "doctor tar check failed", err)
	}
	result.Checks = append(result.Checks, passedDoctorCheck("tar"))

	if err := requireRuntimeServiceHealth(ctx, target, cfg.DBService, cfg.AppServices, rt); err != nil {
		return failDoctorCheck(result, "service_health", ErrorKindRuntime, "doctor service health check failed", err)
	}
	result.Checks = append(result.Checks, passedDoctorCheck("service_health"))

	if err := rt.DBPing(ctx, target); err != nil {
		return failDoctorCheck(result, "db_ping", ErrorKindRuntime, "doctor db ping failed", err)
	}
	result.Checks = append(result.Checks, passedDoctorCheck("db_ping"))

	return result, nil
}

func passedDoctorCheck(name string) DoctorCheck {
	return DoctorCheck{Name: name, OK: true}
}

func failDoctorCheck(result DoctorResult, name, kind, message string, err error) (DoctorResult, error) {
	result.Checks = append(result.Checks, DoctorCheck{Name: name, OK: false})
	return result, &VerifyError{Kind: kind, Message: message, Err: err}
}
