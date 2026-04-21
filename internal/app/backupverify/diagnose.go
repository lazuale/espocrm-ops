package backupverify

import (
	"fmt"
	"strings"

	domainfailure "github.com/lazuale/espocrm-ops/internal/domain/failure"
)

func (s Service) Diagnose(req Request) (report Report, err error) {
	manifestPath := strings.TrimSpace(req.ManifestPath)
	backupRoot := strings.TrimSpace(req.BackupRoot)

	switch {
	case manifestPath != "" && backupRoot != "":
		return report, wrapAppError(domainfailure.Failure{
			Kind: domainfailure.KindValidation,
			Code: "backup_verification_failed",
			Err:  fmt.Errorf("use either manifest_path or backup_root, not both"),
		}, "backup_verification_failed")
	case manifestPath == "" && backupRoot == "":
		return report, wrapAppError(domainfailure.Failure{
			Kind: domainfailure.KindValidation,
			Code: "backup_verification_failed",
			Err:  fmt.Errorf("manifest_path or backup_root is required"),
		}, "backup_verification_failed")
	}

	if backupRoot != "" {
		manifestPath, err = s.latestCompleteManifest(backupRoot)
		if err != nil {
			return report, wrapAppError(err, "backup_verification_failed")
		}
	}

	info, err := s.store.VerifyManifestDetailed(manifestPath)
	if err != nil {
		return report, wrapAppError(err, "backup_verification_failed")
	}

	return Report{
		ManifestPath: info.ManifestPath,
		Scope:        info.Scope,
		CreatedAt:    info.CreatedAt,
		DBBackupPath: info.DBBackupPath,
		FilesPath:    info.FilesPath,
	}, nil
}

func (s Service) latestCompleteManifest(backupRoot string) (string, error) {
	candidates, err := s.store.ManifestCandidates(backupRoot)
	if err != nil {
		return "", err
	}

	for _, candidate := range candidates {
		if _, err := s.store.VerifyManifestDetailed(candidate.ManifestPath); err == nil {
			return candidate.ManifestPath, nil
		}
	}

	return "", domainfailure.Failure{
		Kind: domainfailure.KindValidation,
		Code: "backup_verification_failed",
		Err:  fmt.Errorf("no complete backup set found in %s", backupRoot),
	}
}
