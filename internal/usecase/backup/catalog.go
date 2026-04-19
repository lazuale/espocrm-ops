package backup

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	domainbackup "github.com/lazuale/espocrm-ops/internal/domain/backup"
	domainjournal "github.com/lazuale/espocrm-ops/internal/domain/journal"
	"github.com/lazuale/espocrm-ops/internal/platform/backupstore"
	"github.com/lazuale/espocrm-ops/internal/platform/journalstore"
	journalusecase "github.com/lazuale/espocrm-ops/internal/usecase/journal"
)

const (
	CatalogChecksumMissing        = domainbackup.CatalogChecksumMissing
	CatalogChecksumSidecarMissing = domainbackup.CatalogChecksumSidecarMissing
	CatalogChecksumSidecarPresent = domainbackup.CatalogChecksumSidecarPresent
	CatalogChecksumVerified       = domainbackup.CatalogChecksumVerified
	CatalogChecksumMismatch       = domainbackup.CatalogChecksumMismatch

	CatalogManifestMissing   = domainbackup.CatalogManifestMissing
	CatalogManifestValid     = domainbackup.CatalogManifestValid
	CatalogManifestInvalid   = domainbackup.CatalogManifestInvalid
	CatalogManifestDirectory = domainbackup.CatalogManifestDirectory

	CatalogReadinessReadyVerified   = domainbackup.CatalogReadinessReadyVerified
	CatalogReadinessReadyUnverified = domainbackup.CatalogReadinessReadyUnverified
	CatalogReadinessIncomplete      = domainbackup.CatalogReadinessIncomplete
	CatalogReadinessCorrupted       = domainbackup.CatalogReadinessCorrupted

	BackupOriginUnknown                    = "unknown"
	BackupOriginNormalBackup               = "normal_backup"
	BackupOriginUpdateRecoveryPoint        = "update_recovery_point"
	BackupOriginRollbackProtectionSnapshot = "rollback_protection_snapshot"
	BackupOriginOtherInternalSource        = "other_internal_source"
)

type CatalogRequest struct {
	BackupRoot     string
	JournalDir     string
	VerifyChecksum bool
	ReadyOnly      bool
	Limit          int
	Now            time.Time
}

type CatalogInfo struct {
	BackupRoot      string           `json:"backup_root"`
	VerifyChecksum  bool             `json:"verify_checksum"`
	ReadyOnly       bool             `json:"ready_only"`
	Limit           int              `json:"limit"`
	TotalSets       int              `json:"total_sets"`
	ShownSets       int              `json:"shown_sets"`
	Items           []CatalogItem    `json:"items"`
	JournalRead     JournalReadStats `json:"journal_read"`
	DiscoveredStamp string           `json:"discovered_at,omitempty"`
}

type CatalogItem struct {
	ID               string          `json:"id"`
	Prefix           string          `json:"prefix"`
	Stamp            string          `json:"stamp"`
	GroupKey         string          `json:"group_key"`
	Scope            string          `json:"scope,omitempty"`
	Contour          string          `json:"contour,omitempty"`
	CreatedAt        string          `json:"created_at,omitempty"`
	ComposeProject   string          `json:"compose_project,omitempty"`
	Origin           BackupOrigin    `json:"origin"`
	RestoreReadiness string          `json:"restore_readiness"`
	IsReady          bool            `json:"is_ready"`
	DB               CatalogArtifact `json:"db"`
	Files            CatalogArtifact `json:"files"`
	ManifestTXT      CatalogManifest `json:"manifest_txt"`
	ManifestJSON     CatalogManifest `json:"manifest_json"`
}

type BackupOrigin struct {
	Kind        string `json:"kind"`
	Label       string `json:"label,omitempty"`
	OperationID string `json:"operation_id,omitempty"`
	Command     string `json:"command,omitempty"`
	StartedAt   string `json:"started_at,omitempty"`
}

type JournalReadStats struct {
	TotalFilesSeen int `json:"total_files_seen"`
	LoadedEntries  int `json:"loaded_entries"`
	SkippedCorrupt int `json:"skipped_corrupt"`
}

type CatalogArtifact struct {
	File           string `json:"file,omitempty"`
	Sidecar        string `json:"sidecar,omitempty"`
	AgeHours       *int   `json:"age_hours,omitempty"`
	SizeBytes      *int64 `json:"size_bytes,omitempty"`
	ChecksumStatus string `json:"checksum_status"`
}

type CatalogManifest struct {
	File           string `json:"file,omitempty"`
	AgeHours       *int   `json:"age_hours,omitempty"`
	Status         string `json:"status"`
	Error          string `json:"error,omitempty"`
	Version        int    `json:"version,omitempty"`
	Scope          string `json:"scope,omitempty"`
	Contour        string `json:"contour,omitempty"`
	CreatedAt      string `json:"created_at,omitempty"`
	ComposeProject string `json:"compose_project,omitempty"`
}

type catalogContext struct {
	backupRoot     string
	verifyChecksum bool
	now            time.Time
	originByPath   map[string]BackupOrigin
	journalRead    JournalReadStats
}

type catalogJSONManifest struct {
	Version        int                            `json:"version"`
	Scope          string                         `json:"scope"`
	Contour        string                         `json:"contour"`
	CreatedAt      string                         `json:"created_at"`
	ComposeProject string                         `json:"compose_project"`
	Artifacts      domainbackup.ManifestArtifacts `json:"artifacts"`
	Checksums      domainbackup.ManifestChecksums `json:"checksums"`
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

	ctx, err := buildCatalogContext(backupRoot, strings.TrimSpace(req.JournalDir), req.VerifyChecksum, now)
	if err != nil {
		return CatalogInfo{}, err
	}

	groups, err := catalogGroups(backupRoot)
	if err != nil {
		return CatalogInfo{}, err
	}

	items := make([]CatalogItem, 0, len(groups))
	for _, group := range groups {
		item, err := catalogItem(ctx, group)
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
		JournalRead:     ctx.journalRead,
		DiscoveredStamp: now.UTC().Format(time.RFC3339),
	}, nil
}

func buildCatalogContext(backupRoot, journalDir string, verifyChecksum bool, now time.Time) (catalogContext, error) {
	originByPath, journalRead, err := buildOriginIndex(journalDir)
	if err != nil {
		return catalogContext{}, err
	}

	return catalogContext{
		backupRoot:     backupRoot,
		verifyChecksum: verifyChecksum,
		now:            now,
		originByPath:   originByPath,
		journalRead:    journalRead,
	}, nil
}

func catalogGroups(backupRoot string) ([]domainbackup.BackupGroup, error) {
	return backupstore.Groups(backupRoot, backupstore.GroupModeAny)
}

func catalogItem(ctx catalogContext, group domainbackup.BackupGroup) (CatalogItem, error) {
	set := domainbackup.BuildBackupSet(ctx.backupRoot, group.Prefix, group.Stamp)

	db, err := catalogArtifact(set.DBBackup.Path, ctx.verifyChecksum, ctx.now)
	if err != nil {
		return CatalogItem{}, err
	}
	files, err := catalogArtifact(set.FilesBackup.Path, ctx.verifyChecksum, ctx.now)
	if err != nil {
		return CatalogItem{}, err
	}
	manifestTXT, err := catalogManifest(set.ManifestTXT.Path, ctx.now)
	if err != nil {
		return CatalogItem{}, err
	}
	manifestJSON, err := catalogManifest(set.ManifestJSON.Path, ctx.now)
	if err != nil {
		return CatalogItem{}, err
	}

	item := catalogItemFromDomain(domainbackup.NewCatalogItem(
		group,
		domainbackup.CatalogArtifact(db),
		domainbackup.CatalogArtifact(files),
		domainbackup.CatalogManifest(manifestTXT),
		domainbackup.CatalogManifest(manifestJSON),
		ctx.verifyChecksum,
	))
	item.Origin = originForSet(set, ctx.originByPath)

	return item, nil
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
		ID:               item.ID,
		Prefix:           item.Prefix,
		Stamp:            item.Stamp,
		GroupKey:         item.GroupKey,
		Scope:            item.Scope,
		Contour:          item.Contour,
		CreatedAt:        item.CreatedAt,
		ComposeProject:   item.ComposeProject,
		Origin:           unknownBackupOrigin(),
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
		return CatalogManifest{Status: CatalogManifestMissing}, nil
	}

	age := domainbackup.AgeHours(now, fileInfo.ModTime)
	manifest := CatalogManifest{
		File:     path,
		AgeHours: &age,
	}

	if fileInfo.IsDir {
		manifest.Status = CatalogManifestDirectory
		manifest.Error = "manifest path is a directory"
		return manifest, nil
	}

	if strings.HasSuffix(path, ".manifest.txt") {
		if err := backupstore.ValidateTXTManifest(path); err != nil {
			manifest.Status = CatalogManifestInvalid
			manifest.Error = err.Error()
			return manifest, nil
		}
		manifest.Status = CatalogManifestValid
		return manifest, nil
	}

	metadata, err := loadCatalogJSONManifest(path)
	if err != nil {
		manifest.Status = CatalogManifestInvalid
		manifest.Error = err.Error()
		return manifest, nil
	}

	manifest.Status = CatalogManifestValid
	manifest.Version = metadata.Version
	manifest.Scope = metadata.Scope
	manifest.Contour = firstNonBlank(metadata.Contour, metadata.Scope)
	manifest.CreatedAt = metadata.CreatedAt
	manifest.ComposeProject = metadata.ComposeProject

	return manifest, nil
}

func loadCatalogJSONManifest(path string) (catalogJSONManifest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return catalogJSONManifest{}, fmt.Errorf("read manifest: %w", err)
	}

	var manifest catalogJSONManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return catalogJSONManifest{}, fmt.Errorf("parse manifest json: %w", err)
	}

	if err := (domainbackup.Manifest{
		Version:   manifest.Version,
		Scope:     manifest.Scope,
		CreatedAt: manifest.CreatedAt,
		Artifacts: manifest.Artifacts,
		Checksums: manifest.Checksums,
	}).Validate(); err != nil {
		return catalogJSONManifest{}, fmt.Errorf("validate manifest: %w", err)
	}

	manifest.Contour = strings.TrimSpace(manifest.Contour)
	manifest.ComposeProject = strings.TrimSpace(manifest.ComposeProject)

	return manifest, nil
}

func buildOriginIndex(journalDir string) (map[string]BackupOrigin, JournalReadStats, error) {
	index := map[string]BackupOrigin{}
	if journalDir == "" {
		return index, JournalReadStats{}, nil
	}

	entries, stats, err := (journalstore.Reader{Dir: journalDir}).ReadAll()
	if err != nil {
		return nil, JournalReadStats{}, err
	}

	for _, entry := range entries {
		addReportOrigins(index, journalusecase.Explain(journalusecase.Entry(entry)))
	}

	return index, journalReadStatsFromDomain(stats), nil
}

func addReportOrigins(index map[string]BackupOrigin, report journalusecase.OperationReport) {
	switch report.Command {
	case "backup", "backup-exec":
		recordReportOrigins(index, newBackupOrigin(BackupOriginNormalBackup, report), []string{
			reportArtifact(report, "manifest"),
			reportArtifact(report, "manifest_json"),
			reportArtifact(report, "manifest_txt"),
			reportArtifact(report, "db_backup"),
			reportArtifact(report, "files_backup"),
		})
	case "update", "update-backup":
		recordReportOrigins(index, newBackupOrigin(BackupOriginUpdateRecoveryPoint, report), []string{
			reportArtifact(report, "manifest_json"),
			reportArtifact(report, "manifest_txt"),
			reportArtifact(report, "db_backup"),
			reportArtifact(report, "files_backup"),
		})
	case "rollback":
		recordReportOrigins(index, newBackupOrigin(BackupOriginRollbackProtectionSnapshot, report), []string{
			reportArtifact(report, "snapshot_manifest_json"),
			reportArtifact(report, "snapshot_manifest_txt"),
			reportArtifact(report, "snapshot_db_backup"),
			reportArtifact(report, "snapshot_files_backup"),
		})
	}
}

func recordReportOrigins(index map[string]BackupOrigin, origin BackupOrigin, paths []string) {
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		if _, exists := index[path]; exists {
			continue
		}
		index[path] = origin
	}
}

func newBackupOrigin(kind string, report journalusecase.OperationReport) BackupOrigin {
	return BackupOrigin{
		Kind:        kind,
		Label:       backupOriginLabel(kind),
		OperationID: report.OperationID,
		Command:     report.Command,
		StartedAt:   report.StartedAt,
	}
}

func backupOriginLabel(kind string) string {
	switch kind {
	case BackupOriginNormalBackup:
		return "normal backup"
	case BackupOriginUpdateRecoveryPoint:
		return "update recovery point"
	case BackupOriginRollbackProtectionSnapshot:
		return "rollback protection snapshot"
	case BackupOriginOtherInternalSource:
		return "other internal source"
	default:
		return "unknown"
	}
}

func unknownBackupOrigin() BackupOrigin {
	return BackupOrigin{
		Kind:  BackupOriginUnknown,
		Label: backupOriginLabel(BackupOriginUnknown),
	}
}

func originForSet(set domainbackup.BackupSet, index map[string]BackupOrigin) BackupOrigin {
	for _, path := range []string{
		set.ManifestJSON.Path,
		set.ManifestTXT.Path,
		set.DBBackup.Path,
		set.FilesBackup.Path,
	} {
		if origin, ok := index[path]; ok {
			return origin
		}
	}

	return unknownBackupOrigin()
}

func journalReadStatsFromDomain(stats domainjournal.ReadStats) JournalReadStats {
	return JournalReadStats{
		TotalFilesSeen: stats.TotalFilesSeen,
		LoadedEntries:  stats.LoadedEntries,
		SkippedCorrupt: stats.SkippedCorrupt,
	}
}

func reportArtifact(report journalusecase.OperationReport, key string) string {
	if report.Artifacts == nil {
		return ""
	}

	value, _ := report.Artifacts[key].(string)
	return strings.TrimSpace(value)
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}

	return ""
}
