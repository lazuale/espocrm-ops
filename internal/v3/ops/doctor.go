package ops

import (
	"context"
	"fmt"
	"os"
	"strings"

	v3config "github.com/lazuale/espocrm-ops/internal/v3/config"
	v3runtime "github.com/lazuale/espocrm-ops/internal/v3/runtime"
)

type DoctorCheck struct {
	Name string `json:"name"`
	OK   bool   `json:"ok"`
}

type DoctorResult struct {
	Checks []DoctorCheck `json:"checks"`
}

type doctorRuntime interface {
	ComposeConfig(ctx context.Context, target v3runtime.Target) error
	Services(ctx context.Context, target v3runtime.Target) ([]v3runtime.Service, error)
	DBPing(ctx context.Context, target v3runtime.Target) error
}

func Doctor(ctx context.Context, req v3config.BackupRequest, rt doctorRuntime) (DoctorResult, error) {
	if rt == nil {
		return DoctorResult{}, runtimeError("doctor runtime is required", nil)
	}

	result := DoctorResult{Checks: make([]DoctorCheck, 0, 6)}

	cfg, err := v3config.LoadBackup(req)
	if err != nil {
		return failDoctorCheck(result, "config", ErrorKindUsage, "doctor config check failed", err)
	}
	result.Checks = append(result.Checks, passedDoctorCheck("config"))

	if err := checkDoctorBackupRoot(cfg.BackupRoot); err != nil {
		return failDoctorCheck(result, "backup_root", ErrorKindIO, "doctor backup root check failed", err)
	}
	result.Checks = append(result.Checks, passedDoctorCheck("backup_root"))

	if err := checkDoctorStorageDir(cfg.StorageDir); err != nil {
		return failDoctorCheck(result, "storage_dir", ErrorKindIO, "doctor storage dir check failed", err)
	}
	result.Checks = append(result.Checks, passedDoctorCheck("storage_dir"))

	target := v3runtime.Target{
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

	services, err := rt.Services(ctx, target)
	if err != nil {
		return failDoctorCheck(result, "services", ErrorKindRuntime, "doctor services check failed", err)
	}
	if err := requireDoctorServices(services, cfg.DBService, cfg.AppServices); err != nil {
		return failDoctorCheck(result, "services", ErrorKindRuntime, "doctor services check failed", err)
	}
	result.Checks = append(result.Checks, passedDoctorCheck("services"))

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

func checkDoctorBackupRoot(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("backup root is required")
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return err
	}

	probe, err := os.CreateTemp(path, ".doctor-write-*")
	if err != nil {
		return err
	}
	probePath := probe.Name()
	if err := probe.Close(); err != nil {
		_ = os.Remove(probePath)
		return err
	}
	if err := os.Remove(probePath); err != nil {
		return err
	}

	return nil
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

func requireDoctorServices(services []v3runtime.Service, dbService string, appServices []string) error {
	available := make(map[string]struct{}, len(services))
	for _, service := range services {
		name := strings.TrimSpace(service.Name)
		if name == "" {
			continue
		}
		available[name] = struct{}{}
	}

	dbService = strings.TrimSpace(dbService)
	if _, ok := available[dbService]; !ok {
		return fmt.Errorf("db service %q not found in docker compose ps output", dbService)
	}
	for _, service := range appServices {
		name := strings.TrimSpace(service)
		if _, ok := available[name]; !ok {
			return fmt.Errorf("app service %q not found in docker compose ps output", name)
		}
	}

	return nil
}
