package backup

import (
	"fmt"
	"strings"
	"time"

	domainbackup "github.com/lazuale/espocrm-ops/internal/domain/backup"
	"github.com/lazuale/espocrm-ops/internal/platform/backupstore"
)

const (
	CatalogChecksumMissing        = domainbackup.CatalogChecksumMissing
	CatalogChecksumSidecarMissing = domainbackup.CatalogChecksumSidecarMissing
	CatalogChecksumSidecarPresent = domainbackup.CatalogChecksumSidecarPresent
	CatalogChecksumVerified       = domainbackup.CatalogChecksumVerified
	CatalogChecksumMismatch       = domainbackup.CatalogChecksumMismatch

	CatalogReadinessReadyVerified   = domainbackup.CatalogReadinessReadyVerified
	CatalogReadinessReadyUnverified = domainbackup.CatalogReadinessReadyUnverified
	CatalogReadinessIncomplete      = domainbackup.CatalogReadinessIncomplete
	CatalogReadinessCorrupted       = domainbackup.CatalogReadinessCorrupted
)

type CatalogRequest struct {
	BackupRoot     string
	VerifyChecksum bool
	ReadyOnly      bool
	Limit          int
	Now            time.Time
}

type CatalogInfo struct {
	BackupRoot      string        `json:"backup_root"`
	VerifyChecksum  bool          `json:"verify_checksum"`
	ReadyOnly       bool          `json:"ready_only"`
	Limit           int           `json:"limit"`
	TotalSets       int           `json:"total_sets"`
	ShownSets       int           `json:"shown_sets"`
	Items           []CatalogItem `json:"items"`
	DiscoveredStamp string        `json:"discovered_at,omitempty"`
}

type CatalogItem struct {
	Prefix           string          `json:"prefix"`
	Stamp            string          `json:"stamp"`
	GroupKey         string          `json:"group_key"`
	RestoreReadiness string          `json:"restore_readiness"`
	IsReady          bool            `json:"is_ready"`
	DB               CatalogArtifact `json:"db"`
	Files            CatalogArtifact `json:"files"`
	ManifestTXT      CatalogManifest `json:"manifest_txt"`
	ManifestJSON     CatalogManifest `json:"manifest_json"`
}

type CatalogArtifact struct {
	File           string `json:"file,omitempty"`
	Sidecar        string `json:"sidecar,omitempty"`
	AgeHours       *int   `json:"age_hours,omitempty"`
	SizeBytes      *int64 `json:"size_bytes,omitempty"`
	ChecksumStatus string `json:"checksum_status"`
}

type CatalogManifest struct {
	File     string `json:"file,omitempty"`
	AgeHours *int   `json:"age_hours,omitempty"`
}

func Catalog(req CatalogRequest) (CatalogInfo, error) {
	backupRoot := strings.TrimSpace(req.BackupRoot)
	if backupRoot == "" {
		return CatalogInfo{}, fmt.Errorf("backup root is required")
	}
	if req.Limit < 0 {
		return CatalogInfo{}, fmt.Errorf("limit must be non-negative")
	}
	now := req.Now
	if now.IsZero() {
		now = time.Now()
	}

	groups, err := catalogGroups(backupRoot)
	if err != nil {
		return CatalogInfo{}, err
	}

	items := make([]CatalogItem, 0, len(groups))
	for _, group := range groups {
		item, err := catalogItem(backupRoot, group, req.VerifyChecksum, now)
		if err != nil {
			return CatalogInfo{}, err
		}
		if req.ReadyOnly && !item.IsReady {
			continue
		}
		items = append(items, item)
		if req.Limit > 0 && len(items) >= req.Limit {
			break
		}
	}

	return CatalogInfo{
		BackupRoot:      backupRoot,
		VerifyChecksum:  req.VerifyChecksum,
		ReadyOnly:       req.ReadyOnly,
		Limit:           req.Limit,
		TotalSets:       len(groups),
		ShownSets:       len(items),
		Items:           items,
		DiscoveredStamp: now.UTC().Format(time.RFC3339),
	}, nil
}

func catalogGroups(backupRoot string) ([]domainbackup.BackupGroup, error) {
	return backupstore.Groups(backupRoot, backupstore.GroupModeAny)
}

func catalogItem(backupRoot string, group domainbackup.BackupGroup, verifyChecksum bool, now time.Time) (CatalogItem, error) {
	set := domainbackup.BuildBackupSet(backupRoot, group.Prefix, group.Stamp)
	db, err := catalogArtifact(set.DBBackup.Path, verifyChecksum, now)
	if err != nil {
		return CatalogItem{}, err
	}
	files, err := catalogArtifact(set.FilesBackup.Path, verifyChecksum, now)
	if err != nil {
		return CatalogItem{}, err
	}
	manifestTXT, err := catalogManifest(set.ManifestTXT.Path, now)
	if err != nil {
		return CatalogItem{}, err
	}
	manifestJSON, err := catalogManifest(set.ManifestJSON.Path, now)
	if err != nil {
		return CatalogItem{}, err
	}
	return catalogItemFromDomain(domainbackup.NewCatalogItem(
		group,
		domainbackup.CatalogArtifact(db),
		domainbackup.CatalogArtifact(files),
		domainbackup.CatalogManifest(manifestTXT),
		domainbackup.CatalogManifest(manifestJSON),
		verifyChecksum,
	)), nil
}

func catalogArtifact(path string, verifyChecksum bool, now time.Time) (CatalogArtifact, error) {
	inspection, err := backupstore.InspectBackupArtifact(path, "backup", verifyChecksum)
	if err != nil {
		return CatalogArtifact{}, fmt.Errorf("stat artifact %s: %w", path, err)
	}
	if !inspection.FileInfo.Exists {
		return CatalogArtifact{ChecksumStatus: CatalogChecksumMissing}, nil
	}

	age := domainbackup.AgeHours(now, inspection.FileInfo.ModTime)
	size := inspection.FileInfo.Size
	artifact := CatalogArtifact{
		File:      path,
		Sidecar:   inspection.SidecarPath,
		AgeHours:  &age,
		SizeBytes: &size,
	}
	if inspection.FileInfo.IsDir {
		artifact.ChecksumStatus = CatalogChecksumMismatch
		return artifact, nil
	}

	if !inspection.SidecarInfo.Exists {
		artifact.Sidecar = ""
		artifact.ChecksumStatus = CatalogChecksumSidecarMissing
		return artifact, nil
	}
	if inspection.ChecksumError != nil {
		artifact.ChecksumStatus = CatalogChecksumMismatch
		return artifact, nil
	}
	if inspection.ChecksumVerified {
		artifact.ChecksumStatus = CatalogChecksumVerified
		return artifact, nil
	}
	if !verifyChecksum {
		artifact.ChecksumStatus = CatalogChecksumSidecarPresent
		return artifact, nil
	}

	artifact.ChecksumStatus = CatalogChecksumMismatch
	return artifact, nil
}

func catalogItemFromDomain(item domainbackup.CatalogItem) CatalogItem {
	return CatalogItem{
		Prefix:           item.Prefix,
		Stamp:            item.Stamp,
		GroupKey:         item.GroupKey,
		RestoreReadiness: item.RestoreReadiness,
		IsReady:          item.IsReady,
		DB:               CatalogArtifact(item.DB),
		Files:            CatalogArtifact(item.Files),
		ManifestTXT:      CatalogManifest(item.ManifestTXT),
		ManifestJSON:     CatalogManifest(item.ManifestJSON),
	}
}

func catalogManifest(path string, now time.Time) (CatalogManifest, error) {
	fileInfo, err := backupstore.InspectFile(path)
	if err != nil {
		return CatalogManifest{}, fmt.Errorf("stat manifest %s: %w", path, err)
	}
	if !fileInfo.Exists {
		return CatalogManifest{}, nil
	}
	if fileInfo.IsDir {
		return CatalogManifest{}, fmt.Errorf("manifest is directory: %s", path)
	}
	age := domainbackup.AgeHours(now, fileInfo.ModTime)

	return CatalogManifest{
		File:     path,
		AgeHours: &age,
	}, nil
}
