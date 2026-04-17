package restore

import (
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

func PlanDBRestore(req RestoreDBRequest) (DBRestorePlan, error) {
	prepared, err := prepareDBRestore(req)
	if err != nil {
		return DBRestorePlan{}, err
	}
	defer prepared.lock.Release()

	return prepared.plan, nil
}

func RestoreDB(req RestoreDBRequest) (DBRestorePlan, error) {
	prepared, err := prepareDBRestore(req)
	if err != nil {
		return DBRestorePlan{}, err
	}
	defer prepared.lock.Release()

	if req.DryRun {
		return prepared.plan, nil
	}

	if err := platformdocker.ResetAndRestoreMySQLDumpGz(prepared.dbPath, req.DBContainer, prepared.rootPassword, req.DBName, req.DBUser); err != nil {
		return DBRestorePlan{}, OperationError{Err: err}
	}

	return prepared.plan, nil
}

func prepareDBRestore(req RestoreDBRequest) (preparedDBRestore, error) {
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

	dbPath, err := PreflightDBRestore(DBPreflightRequest{
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
