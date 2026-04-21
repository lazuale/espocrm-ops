package restoreflow

import (
	"errors"
	"fmt"

	domainfailure "github.com/lazuale/espocrm-ops/internal/domain/failure"
)

type preparedDBRestore struct {
	plan         DBPlan
	dbPath       string
	rootPassword string
	lock         restoreLock
}

func (s Service) PlanDB(req DBRequest) (plan DBPlan, err error) {
	prepared, err := s.prepareDB(req)
	if err != nil {
		return DBPlan{}, err
	}
	defer func() {
		if releaseErr := releaseLockError(domainfailure.KindIO, "restore_db_failed", "db restore lock", prepared.lock.Release()); releaseErr != nil {
			if err == nil {
				err = releaseErr
			} else {
				err = errors.Join(err, releaseErr)
			}
		}
	}()

	return prepared.plan, nil
}

func (s Service) RestoreDB(req DBRequest) (plan DBPlan, err error) {
	prepared, err := s.prepareDB(req)
	if err != nil {
		return DBPlan{}, err
	}
	defer func() {
		if releaseErr := releaseLockError(domainfailure.KindIO, "restore_db_failed", "db restore lock", prepared.lock.Release()); releaseErr != nil {
			if err == nil {
				err = releaseErr
			} else {
				err = errors.Join(err, releaseErr)
			}
		}
	}()

	if req.DryRun {
		return prepared.plan, nil
	}

	if err := s.runtime.ResetAndRestoreMySQLDumpGz(prepared.dbPath, req.DBContainer, prepared.rootPassword, req.DBName, req.DBUser); err != nil {
		return DBPlan{}, failure(domainfailure.KindExternal, "restore_db_failed", err)
	}

	return prepared.plan, nil
}

func (s Service) prepareDB(req DBRequest) (preparedDBRestore, error) {
	if err := req.Validate(); err != nil {
		return preparedDBRestore{}, err
	}

	if _, err := s.resolveDBPassword(req); err != nil {
		return preparedDBRestore{}, failure(domainfailure.KindValidation, "preflight_failed", fmt.Errorf("resolve db password: %w", err))
	}

	dbPath, err := s.PreflightDB(DBPreflightRequest{
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
		return preparedDBRestore{}, failure(domainfailure.KindConflict, "lock_acquire_failed", fmt.Errorf("db restore lock failed: %w", err))
	}

	return preparedDBRestore{
		plan:         buildDBPlan(req, dbPath, rootPasswordCheck),
		dbPath:       dbPath,
		rootPassword: rootPassword,
		lock:         lock,
	}, nil
}
