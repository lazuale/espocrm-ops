package backup

import (
	"strings"
	"time"
)

const (
	CatalogChecksumMissing        = "missing"
	CatalogChecksumSidecarMissing = "sidecar_missing"
	CatalogChecksumSidecarPresent = "sidecar_present"
	CatalogChecksumVerified       = "verified"
	CatalogChecksumMismatch       = "mismatch"

	CatalogManifestMissing   = "missing"
	CatalogManifestValid     = "valid"
	CatalogManifestInvalid   = "invalid"
	CatalogManifestDirectory = "directory"

	CatalogReadinessReadyVerified   = "ready_verified"
	CatalogReadinessReadyUnverified = "ready_unverified"
	CatalogReadinessIncomplete      = "incomplete"
	CatalogReadinessCorrupted       = "corrupted"
)

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
	ID               string          `json:"id"`
	Prefix           string          `json:"prefix"`
	Stamp            string          `json:"stamp"`
	GroupKey         string          `json:"group_key"`
	Scope            string          `json:"scope,omitempty"`
	Contour          string          `json:"contour,omitempty"`
	CreatedAt        string          `json:"created_at,omitempty"`
	ComposeProject   string          `json:"compose_project,omitempty"`
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

func NewCatalogItem(group BackupGroup, db, files CatalogArtifact, manifestTXT, manifestJSON CatalogManifest, verifyChecksum bool) CatalogItem {
	readiness := CatalogReadiness(db, files, manifestTXT, manifestJSON, verifyChecksum)
	createdAt := manifestJSON.CreatedAt
	if createdAt == "" {
		createdAt = stampToRFC3339(group.Stamp)
	}

	return CatalogItem{
		ID:               BackupSetID(group),
		Prefix:           group.Prefix,
		Stamp:            group.Stamp,
		GroupKey:         GroupKey(group),
		Scope:            manifestJSON.Scope,
		Contour:          firstNonBlank(manifestJSON.Contour, manifestJSON.Scope),
		CreatedAt:        createdAt,
		ComposeProject:   manifestJSON.ComposeProject,
		RestoreReadiness: readiness,
		IsReady:          IsCatalogReady(readiness),
		DB:               db,
		Files:            files,
		ManifestTXT:      manifestTXT,
		ManifestJSON:     manifestJSON,
	}
}

func CatalogReadiness(db, files CatalogArtifact, manifestTXT, manifestJSON CatalogManifest, verifyChecksum bool) string {
	if db.ChecksumStatus == CatalogChecksumMismatch ||
		files.ChecksumStatus == CatalogChecksumMismatch ||
		manifestTXT.Status == CatalogManifestInvalid ||
		manifestTXT.Status == CatalogManifestDirectory ||
		manifestJSON.Status == CatalogManifestInvalid ||
		manifestJSON.Status == CatalogManifestDirectory {
		return CatalogReadinessCorrupted
	}
	if db.File == "" ||
		db.Sidecar == "" ||
		files.File == "" ||
		files.Sidecar == "" ||
		manifestTXT.File == "" ||
		manifestTXT.Status != CatalogManifestValid ||
		manifestJSON.File == "" ||
		manifestJSON.Status != CatalogManifestValid {
		return CatalogReadinessIncomplete
	}
	if verifyChecksum {
		return CatalogReadinessReadyVerified
	}

	return CatalogReadinessReadyUnverified
}

func IsCatalogReady(readiness string) bool {
	return strings.HasPrefix(readiness, "ready_")
}

func AgeHours(now, modTime time.Time) int {
	if now.Before(modTime) {
		return 0
	}

	return int(now.Sub(modTime).Hours())
}

func GroupKey(group BackupGroup) string {
	return group.Prefix + "|" + group.Stamp
}

func stampToRFC3339(stamp string) string {
	parsed, err := time.Parse("2006-01-02_15-04-05", stamp)
	if err != nil {
		return ""
	}

	return parsed.UTC().Format(time.RFC3339)
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}

	return ""
}
