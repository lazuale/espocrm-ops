package backupstoreadapter

import (
	"errors"

	backupstoreport "github.com/lazuale/espocrm-ops/internal/app/ports/backupstoreport"
	domainbackup "github.com/lazuale/espocrm-ops/internal/domain/backup"
	domainfailure "github.com/lazuale/espocrm-ops/internal/domain/failure"
	platformbackupstore "github.com/lazuale/espocrm-ops/internal/platform/backupstore"
)

type BackupStore struct{}

func (BackupStore) VerifyManifestDetailed(manifestPath string) (backupstoreport.VerifiedBackup, error) {
	info, err := platformbackupstore.VerifyManifestDetailed(manifestPath)
	if err != nil {
		return backupstoreport.VerifiedBackup{}, classifyBackupStoreError(err)
	}
	return backupstoreport.VerifiedBackup{
		ManifestPath: info.ManifestPath,
		Scope:        info.Scope,
		CreatedAt:    info.CreatedAt,
		DBBackupPath: info.DBBackupPath,
		FilesPath:    info.FilesPath,
	}, nil
}

func (BackupStore) VerifyDirectDBBackup(dbPath string) error {
	return classifyBackupStoreError(platformbackupstore.VerifyDirectDBBackup(dbPath))
}

func (BackupStore) VerifyDirectFilesBackup(filesPath string) error {
	return classifyBackupStoreError(platformbackupstore.VerifyDirectFilesBackup(filesPath))
}

func (BackupStore) ManifestCandidates(backupRoot string) ([]backupstoreport.ManifestCandidate, error) {
	candidates, err := platformbackupstore.ManifestCandidates(backupRoot)
	if err != nil {
		return nil, err
	}
	out := make([]backupstoreport.ManifestCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		out = append(out, backupstoreport.ManifestCandidate{
			Prefix:       candidate.Prefix,
			Stamp:        candidate.Stamp,
			ManifestPath: candidate.ManifestPath,
		})
	}
	return out, nil
}

func (BackupStore) Groups(backupRoot string, mode backupstoreport.GroupMode) ([]domainbackup.BackupGroup, error) {
	return platformbackupstore.Groups(backupRoot, platformbackupstore.GroupMode(mode))
}

func (BackupStore) LoadManifest(path string) (domainbackup.Manifest, error) {
	manifest, err := platformbackupstore.LoadManifest(path)
	if err != nil {
		return domainbackup.Manifest{}, classifyBackupStoreError(err)
	}
	return manifest, nil
}

func (BackupStore) WriteManifest(path string, manifest domainbackup.Manifest) error {
	return platformbackupstore.WriteManifest(path, manifest)
}

func (BackupStore) WriteSHA256Sidecar(filePath, checksum, sidecarPath string) error {
	return platformbackupstore.WriteSHA256Sidecar(filePath, checksum, sidecarPath)
}

func classifyBackupStoreError(err error) error {
	if err == nil {
		return nil
	}

	var manifestErr platformbackupstore.ManifestError
	if errors.As(err, &manifestErr) {
		return domainfailure.Failure{Kind: domainfailure.KindManifest, Code: "manifest_invalid", Err: err}
	}

	var verificationErr platformbackupstore.VerificationError
	if errors.As(err, &verificationErr) {
		return domainfailure.Failure{Kind: domainfailure.KindValidation, Code: "backup_verification_failed", Err: err}
	}

	return err
}
