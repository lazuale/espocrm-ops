package backupstoreport

import domainbackup "github.com/lazuale/espocrm-ops/internal/domain/backup"

type VerifiedBackup struct {
	ManifestPath string
	Scope        string
	CreatedAt    string
	DBBackupPath string
	FilesPath    string
}

type ManifestCandidate struct {
	Prefix       string
	Stamp        string
	ManifestPath string
}

type GroupMode int

const (
	GroupModeAny GroupMode = iota
	GroupModeDB
	GroupModeFiles
	GroupModeManifests
)

type Store interface {
	VerifyManifest(manifestPath string) error
	VerifyManifestDetailed(manifestPath string) (VerifiedBackup, error)
	VerifyDirectDBBackup(dbPath string) error
	VerifyDirectFilesBackup(filesPath string) error
	ManifestCandidates(backupRoot string) ([]ManifestCandidate, error)
	Groups(backupRoot string, mode GroupMode) ([]domainbackup.BackupGroup, error)
	LoadManifest(path string) (domainbackup.Manifest, error)
	WriteManifest(path string, manifest domainbackup.Manifest) error
	WriteSHA256Sidecar(filePath, checksum, sidecarPath string) error
}
