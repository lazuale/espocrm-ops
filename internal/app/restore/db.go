package restore

import (
	"errors"
	"fmt"

	domainfailure "github.com/lazuale/espocrm-ops/internal/domain/failure"
)

type preparedDBRestore struct {
	plan         DBRestorePlan
	dbPath       string
	rootPassword string
	lock         restoreLock
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

	return prepared.plan, nil
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
		return prepared.plan, nil
	}

	if err := s.runtime.ResetAndRestoreMySQLDumpGz(prepared.dbPath, req.DBContainer, prepared.rootPassword, req.DBName, req.DBUser); err != nil {
		return DBRestorePlan{}, restoreFailure(domainfailure.KindExternal, "restore_db_failed", err)
	}

	return prepared.plan, nil
}

func (s Service) prepareDBRestore(req RestoreDBRequest) (preparedDBRestore, error) {
	if err := req.Validate(); err != nil {
		return preparedDBRestore{}, err
	}

	if _, err := s.resolveDBPassword(req); err != nil {
		return preparedDBRestore{}, restoreFailure(domainfailure.KindValidation, "preflight_failed", fmt.Errorf("resolve db password: %w", err))
	}

	dbPath, err := s.PreflightDBRestore(DBPreflightRequest{
		ManifestPath: req.ManifestPath,
		DBBackup:     req.DBBackup,
		DBContainer:  req.DBContainer,
	})
	if err != nil {
		return preparedDBRestore{}, err
	}

	rootPassword, rootPasswordCheck, err := s.resolveDBRootPasswordForPlan(req)
	if err != nil {
		return preparedDBRestore{}, err
	}

	lock, err := s.locks.AcquireRestoreDBLock()
	if err != nil {
		return preparedDBRestore{}, restoreFailure(domainfailure.KindConflict, "lock_acquire_failed", fmt.Errorf("db restore lock failed: %w", err))
	}

	return preparedDBRestore{
		plan:         buildDBRestorePlan(req, dbPath, rootPasswordCheck),
		dbPath:       dbPath,
		rootPassword: rootPassword,
		lock:         lock,
	}, nil
}
