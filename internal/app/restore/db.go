package restore

import (
	"errors"
	"fmt"

	"github.com/lazuale/espocrm-ops/internal/platform/config"
	platformdocker "github.com/lazuale/espocrm-ops/internal/platform/docker"
	platformlocks "github.com/lazuale/espocrm-ops/internal/platform/locks"
)

type preparedDBRestore struct {
	plan         DBRestorePlan
	dbPath       string
	rootPassword string
	lock         *platformlocks.FileLock
}

func (s Service) PlanDBRestore(req RestoreDBRequest) (plan DBRestorePlan, err error) {
	prepared, err := s.prepareDBRestore(req)
	if err != nil {
		return DBRestorePlan{}, err
	}
	defer func() {
		if releaseErr := prepared.lock.Release(); releaseErr != nil {
			wrapped := fmt.Errorf("release db restore lock: %w", releaseErr)
			if err == nil {
				err = wrapped
			} else {
				err = errors.Join(err, wrapped)
			}
		}
	}()

	plan = prepared.plan
	return plan, nil
}

func (s Service) RestoreDB(req RestoreDBRequest) (plan DBRestorePlan, err error) {
	prepared, err := s.prepareDBRestore(req)
	if err != nil {
		return DBRestorePlan{}, err
	}
	defer func() {
		if releaseErr := prepared.lock.Release(); releaseErr != nil {
			wrapped := fmt.Errorf("release db restore lock: %w", releaseErr)
			if err == nil {
				err = wrapped
			} else {
				err = errors.Join(err, wrapped)
			}
		}
	}()

	if req.DryRun {
		plan = prepared.plan
		return plan, nil
	}

	if err := platformdocker.ResetAndRestoreMySQLDumpGz(prepared.dbPath, req.DBContainer, prepared.rootPassword, req.DBName, req.DBUser); err != nil {
		return DBRestorePlan{}, OperationError{Err: err}
	}

	plan = prepared.plan
	return plan, nil
}

func (s Service) prepareDBRestore(req RestoreDBRequest) (preparedDBRestore, error) {
	if err := req.Validate(); err != nil {
		return preparedDBRestore{}, err
	}

	if _, err := config.ResolveDBPassword(config.DBConfig{
		Container:    req.DBContainer,
		Name:         req.DBName,
		User:         req.DBUser,
		Password:     req.DBPassword,
		PasswordFile: req.DBPasswordFile,
	}); err != nil {
		return preparedDBRestore{}, PreflightError{Err: fmt.Errorf("resolve db password: %w", err)}
	}

	dbPath, err := s.PreflightDBRestore(DBPreflightRequest{
		ManifestPath: req.ManifestPath,
		DBBackup:     req.DBBackup,
		DBContainer:  req.DBContainer,
	})
	if err != nil {
		return preparedDBRestore{}, err
	}

	rootPassword, rootPasswordCheck, err := resolveDBRootPasswordForPlan(req)
	if err != nil {
		return preparedDBRestore{}, err
	}

	lock, err := platformlocks.AcquireRestoreDBLock()
	if err != nil {
		return preparedDBRestore{}, LockError{Err: fmt.Errorf("db restore lock failed: %w", err)}
	}

	return preparedDBRestore{
		plan:         buildDBRestorePlan(req, dbPath, rootPasswordCheck),
		dbPath:       dbPath,
		rootPassword: rootPassword,
		lock:         lock,
	}, nil
}
