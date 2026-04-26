package ops

import (
	"context"
	"fmt"
	"os"
	"strings"

	config "github.com/lazuale/espocrm-ops/internal/config"
	runtime "github.com/lazuale/espocrm-ops/internal/runtime"
)

type DoctorCheck struct {
	Name string `json:"name"`
	OK   bool   `json:"ok"`
}

type DoctorResult struct {
	Checks   []DoctorCheck `json:"checks"`
	Warnings []string      `json:"-"`
}

type doctorRuntime interface {
	ComposeConfig(ctx context.Context, target runtime.Target) error
	RequireHealthyServices(ctx context.Context, target runtime.Target, services []string) error
	DBPing(ctx context.Context, target runtime.Target) error
}

func Doctor(ctx context.Context, req config.BackupRequest, rt doctorRuntime) (DoctorResult, error) {
	if rt == nil {
		return DoctorResult{}, runtimeError("doctor runtime is required", nil)
	}

	result := DoctorResult{Checks: make([]DoctorCheck, 0, 6)}

	cfg, err := config.LoadBackup(req)
	if err != nil {
		return failDoctorCheck(result, "config", ErrorKindUsage, "doctor config check failed", err)
	}
	result.Warnings = append(result.Warnings, cfg.Warnings...)
	result.Checks = append(result.Checks, passedDoctorCheck("config"))

	backupRootWarnings, err := checkDoctorBackupRoot(cfg.BackupRoot)
	if err != nil {
		return failDoctorCheck(result, "backup_root", ErrorKindIO, "doctor backup root check failed", err)
	}
	result.Warnings = append(result.Warnings, backupRootWarnings...)
	result.Checks = append(result.Checks, passedDoctorCheck("backup_root"))

	if err := checkDoctorStorageDir(cfg.StorageDir); err != nil {
		return failDoctorCheck(result, "storage_dir", ErrorKindIO, "doctor storage dir check failed", err)
	}
	result.Checks = append(result.Checks, passedDoctorCheck("storage_dir"))

	target := runtime.Target{
		ProjectDir:  cfg.ProjectDir,
		ComposeFile: cfg.ComposeFile,
		EnvFile:     cfg.EnvFile,
		DBService:   cfg.DBService,
		DBUser:      cfg.DBUser,
		DBPassword:  cfg.DBPassword,
		DBName:      cfg.DBName,
	}

	if err := rt.ComposeConfig(ctx, target); err != nil {
		return failDoctorCheck(result, "compose_config", ErrorKindRuntime, "doctor compose config check failed", err)
	}
	result.Checks = append(result.Checks, passedDoctorCheck("compose_config"))

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

func checkDoctorBackupRoot(path string) ([]string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("backup root is required")
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("backup root must be a directory")
	}

	probe, err := os.CreateTemp(path, ".doctor-write-*")
	if err != nil {
		return []string{fmt.Sprintf("backup root %s is not writable by the operator account: %v", path, err)}, nil
	}
	probePath := probe.Name()
	if err := probe.Close(); err != nil {
		_ = os.Remove(probePath)
		return []string{fmt.Sprintf("backup root %s write probe close failed: %v", path, err)}, nil
	}
	if err := os.Remove(probePath); err != nil {
		return []string{fmt.Sprintf("backup root %s write probe cleanup failed: %v", path, err)}, nil
	}

	return nil, nil
}

func checkDoctorStorageDir(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("storage dir is required")
	}

	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("storage dir root must not be a symlink")
	}
	if !info.IsDir() {
		return fmt.Errorf("storage dir must be a directory")
	}
	if _, err := os.ReadDir(path); err != nil {
		return err
	}

	return nil
}
