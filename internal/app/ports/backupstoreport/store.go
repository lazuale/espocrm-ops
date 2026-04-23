package backupstoreport

import domainbackup "github.com/lazuale/espocrm-ops/internal/domain/backup"

type VerifiedBackup struct {
	ManifestPath string
	Scope        string
	CreatedAt    string
	DBBackupPath string
	FilesPath    string
}

type GroupMode int

const (
	GroupModeAny GroupMode = iota
	GroupModeDB
	GroupModeFiles
)

type Store interface {
	VerifyManifestSelection(manifestPath string, needDB, needFiles bool) (VerifiedBackup, error)
	// Сохранено для migrate: CLI `backup verify` больше не использует legacy port.
	VerifyManifestDetailed(manifestPath string) (VerifiedBackup, error)
	VerifyDirectDBBackup(dbPath string) error
	VerifyDirectFilesBackup(filesPath string) error
	Groups(backupRoot string, mode GroupMode) ([]domainbackup.BackupGroup, error)
	LoadManifest(path string) (domainbackup.Manifest, error)
	WriteManifest(path string, manifest domainbackup.Manifest) error
	WriteSHA256Sidecar(filePath, checksum, sidecarPath string) error
}
