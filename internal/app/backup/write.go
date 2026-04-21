package backup

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	domainbackup "github.com/lazuale/espocrm-ops/internal/domain/backup"
)

type ManifestBuildRequest struct {
	Scope           string
	CreatedAt       time.Time
	DBBackupPath    string
	FilesBackupPath string
}

type FinalizeRequest struct {
	Scope            string
	CreatedAt        time.Time
	DBBackupPath     string
	FilesBackupPath  string
	ManifestPath     string
	DBSidecarPath    string
	FilesSidecarPath string
}

type FinalizeInfo struct {
	ManifestPath     string
	DBBackupPath     string
	FilesBackupPath  string
	DBSidecarPath    string
	FilesSidecarPath string
	Scope            string
	CreatedAt        string
}

func (s Service) FinalizeBackup(req FinalizeRequest) (FinalizeInfo, error) {
	if strings.TrimSpace(req.ManifestPath) == "" {
		return FinalizeInfo{}, fmt.Errorf("manifest path is required")
	}

	manifest, err := s.BuildManifest(ManifestBuildRequest{
		Scope:           req.Scope,
		CreatedAt:       req.CreatedAt,
		DBBackupPath:    req.DBBackupPath,
		FilesBackupPath: req.FilesBackupPath,
	})
	if err != nil {
		return FinalizeInfo{}, err
	}

	dbSidecarPath := strings.TrimSpace(req.DBSidecarPath)
	if dbSidecarPath == "" {
		dbSidecarPath = req.DBBackupPath + ".sha256"
	}
	filesSidecarPath := strings.TrimSpace(req.FilesSidecarPath)
	if filesSidecarPath == "" {
		filesSidecarPath = req.FilesBackupPath + ".sha256"
	}
	if err := s.store.WriteSHA256Sidecar(req.DBBackupPath, manifest.Checksums.DBBackup, dbSidecarPath); err != nil {
		return FinalizeInfo{}, fmt.Errorf("write db sha256 sidecar: %w", err)
	}
	if err := s.store.WriteSHA256Sidecar(req.FilesBackupPath, manifest.Checksums.FilesBackup, filesSidecarPath); err != nil {
		return FinalizeInfo{}, fmt.Errorf("write files sha256 sidecar: %w", err)
	}
	if err := s.store.WriteManifest(req.ManifestPath, manifest); err != nil {
		return FinalizeInfo{}, err
	}

	return FinalizeInfo{
		ManifestPath:     req.ManifestPath,
		DBBackupPath:     req.DBBackupPath,
		FilesBackupPath:  req.FilesBackupPath,
		DBSidecarPath:    dbSidecarPath,
		FilesSidecarPath: filesSidecarPath,
		Scope:            manifest.Scope,
		CreatedAt:        manifest.CreatedAt,
	}, nil
}

func (s Service) BuildManifest(req ManifestBuildRequest) (domainbackup.Manifest, error) {
	if strings.TrimSpace(req.Scope) == "" {
		return domainbackup.Manifest{}, fmt.Errorf("scope is required")
	}
	if req.CreatedAt.IsZero() {
		return domainbackup.Manifest{}, fmt.Errorf("created_at is required")
	}
	if strings.TrimSpace(req.DBBackupPath) == "" {
		return domainbackup.Manifest{}, fmt.Errorf("db backup path is required")
	}
	if strings.TrimSpace(req.FilesBackupPath) == "" {
		return domainbackup.Manifest{}, fmt.Errorf("files backup path is required")
	}

	dbChecksum, err := s.files.SHA256File(req.DBBackupPath)
	if err != nil {
		return domainbackup.Manifest{}, fmt.Errorf("hash db backup: %w", err)
	}
	filesChecksum, err := s.files.SHA256File(req.FilesBackupPath)
	if err != nil {
		return domainbackup.Manifest{}, fmt.Errorf("hash files backup: %w", err)
	}

	return domainbackup.BuildManifest(domainbackup.ManifestBuildRequest{
		Scope:           req.Scope,
		CreatedAt:       req.CreatedAt,
		DBBackupName:    filepath.Base(req.DBBackupPath),
		FilesBackupName: filepath.Base(req.FilesBackupPath),
		DBChecksum:      dbChecksum,
		FilesChecksum:   filesChecksum,
	})
}
