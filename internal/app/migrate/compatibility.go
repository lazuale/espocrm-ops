package migrate

import (
	"errors"
	"fmt"
	"strings"

	"github.com/lazuale/espocrm-ops/internal/contract/apperr"
	domainenv "github.com/lazuale/espocrm-ops/internal/domain/env"
	domainfailure "github.com/lazuale/espocrm-ops/internal/domain/failure"
	platformconfig "github.com/lazuale/espocrm-ops/internal/platform/config"
)

func requireMigrationCompatibility(sourceEnv, targetEnv domainenv.OperationEnv, sourceScope, targetScope string) error {
	rawMismatches := domainenv.MigrationCompatibilityMismatches(sourceEnv, targetEnv)
	if len(rawMismatches) == 0 {
		return nil
	}

	mismatches := make([]string, 0, len(rawMismatches))
	for _, mismatch := range rawMismatches {
		mismatches = append(mismatches, fmt.Sprintf("%s ('%s' vs '%s')", mismatch.Name, mismatch.LeftValue, mismatch.RightValue))
	}

	return executeFailure{
		Kind:    domainfailure.KindValidation,
		Summary: "Migration compatibility contract failed",
		Action:  "Align the shared settings first and rerun espops doctor --scope all --project-dir <repo>.",
		Err: fmt.Errorf(
			"configs %q and %q conflict with the migration compatibility contract: %s",
			sourceScope,
			targetScope,
			strings.Join(mismatches, "; "),
		),
	}
}

func classifyMigrationEnvError(err error) error {
	switch err.(type) {
	case platformconfig.MissingEnvFileError, platformconfig.InvalidEnvFileError, platformconfig.EnvParseError, platformconfig.MissingEnvValueError, platformconfig.UnsupportedContourError:
		return executeFailure{Kind: domainfailure.KindValidation, Err: err}
	default:
		return executeFailure{Kind: domainfailure.KindIO, Err: err}
	}
}

func wrapExecuteError(err error) error {
	var failure executeFailure
	if errors.As(err, &failure) && failure.Kind != "" {
		return apperr.Wrap(apperr.Kind(failure.Kind), "migrate_failed", err)
	}
	if kind, ok := apperr.KindOf(err); ok {
		return apperr.Wrap(kind, "migrate_failed", err)
	}

	return apperr.Wrap(apperr.KindInternal, "migrate_failed", err)
}
