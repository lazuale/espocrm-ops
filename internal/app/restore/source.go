package restore

import (
	"fmt"
	"path/filepath"
	"strings"

	domainbackup "github.com/lazuale/espocrm-ops/internal/domain/backup"
	domainfailure "github.com/lazuale/espocrm-ops/internal/domain/failure"
)

func (s Service) resolveExecuteSource(backupRoot string, req ExecuteRequest) (executeSource, error) {
	manifestPath := strings.TrimSpace(req.ManifestPath)
	dbBackup := strings.TrimSpace(req.DBBackup)
	filesBackup := strings.TrimSpace(req.FilesBackup)

	if manifestPath != "" {
		info, err := s.store.VerifyManifestDetailed(manifestPath)
		if err != nil {
			return executeSource{}, executeFailure{
				Kind:    domainfailure.KindValidation,
				Summary: "The selected restore manifest is not valid",
				Action:  "Choose a valid manifest JSON that references readable, verified backup artifacts.",
				Err:     err,
			}
		}
		return executeSource{
			SelectionMode: manifestSelectionMode(req),
			SourceKind:    "manifest",
			ManifestJSON:  filepath.Clean(manifestPath),
			ManifestTXT:   matchingManifestTXT(manifestPath),
			DBBackup:      info.DBBackupPath,
			FilesBackup:   info.FilesPath,
		}, nil
	}

	switch {
	case req.SkipDB:
		filesBackup = filepath.Clean(filesBackup)
		if err := s.store.VerifyDirectFilesBackup(filesBackup); err != nil {
			return executeSource{}, executeFailure{
				Kind:    domainfailure.KindValidation,
				Summary: "The selected files backup is not valid",
				Action:  "Choose a readable .tar.gz files backup with a valid .sha256 sidecar.",
				Err:     err,
			}
		}
		return executeSource{
			SelectionMode: "direct_files_only",
			SourceKind:    "direct",
			FilesBackup:   filesBackup,
		}, nil
	case req.SkipFiles:
		dbBackup = filepath.Clean(dbBackup)
		if err := s.store.VerifyDirectDBBackup(dbBackup); err != nil {
			return executeSource{}, executeFailure{
				Kind:    domainfailure.KindValidation,
				Summary: "The selected database backup is not valid",
				Action:  "Choose a readable .sql.gz database backup with a valid .sha256 sidecar.",
				Err:     err,
			}
		}
		return executeSource{
			SelectionMode: "direct_db_only",
			SourceKind:    "direct",
			DBBackup:      dbBackup,
		}, nil
	default:
		dbBackup = filepath.Clean(dbBackup)
		filesBackup = filepath.Clean(filesBackup)
		if err := s.store.VerifyDirectDBBackup(dbBackup); err != nil {
			return executeSource{}, executeFailure{
				Kind:    domainfailure.KindValidation,
				Summary: "The selected database backup is not valid",
				Action:  "Choose a readable .sql.gz database backup with a valid .sha256 sidecar.",
				Err:     err,
			}
		}
		if err := s.store.VerifyDirectFilesBackup(filesBackup); err != nil {
			return executeSource{}, executeFailure{
				Kind:    domainfailure.KindValidation,
				Summary: "The selected files backup is not valid",
				Action:  "Choose a readable .tar.gz files backup with a valid .sha256 sidecar.",
				Err:     err,
			}
		}
		if err := validateDirectPair(dbBackup, filesBackup); err != nil {
			return executeSource{}, err
		}
		return executeSource{
			SelectionMode: "direct_pair",
			SourceKind:    "direct",
			DBBackup:      dbBackup,
			FilesBackup:   filesBackup,
		}, nil
	}
}

func validateDirectPair(dbPath, filesPath string) error {
	dbGroup, err := domainbackup.ParseDBBackupName(dbPath)
	if err != nil {
		return executeFailure{
			Summary: "The selected database backup name is unsupported",
			Action:  "Choose a canonical .sql.gz backup path or use a manifest-backed restore.",
			Err:     err,
		}
	}
	filesGroup, err := domainbackup.ParseFilesBackupName(filesPath)
	if err != nil {
		return executeFailure{
			Summary: "The selected files backup name is unsupported",
			Action:  "Choose a canonical .tar.gz backup path or use a manifest-backed restore.",
			Err:     err,
		}
	}
	if dbGroup != filesGroup {
		return executeFailure{
			Summary: "The selected database and files backups do not belong to the same backup set",
			Action:  "Pass database and files backups from the same backup set or use a manifest-backed restore.",
			Err:     fmt.Errorf("database backup resolves to %s %s, but files backup resolves to %s %s", dbGroup.Prefix, dbGroup.Stamp, filesGroup.Prefix, filesGroup.Stamp),
		}
	}

	return nil
}

func matchingManifestTXT(manifestJSON string) string {
	if !strings.HasSuffix(manifestJSON, ".manifest.json") {
		return ""
	}
	return strings.TrimSuffix(manifestJSON, ".manifest.json") + ".manifest.txt"
}

func manifestSelectionMode(req ExecuteRequest) string {
	switch {
	case req.SkipDB:
		return "manifest_files_only"
	case req.SkipFiles:
		return "manifest_db_only"
	default:
		return "manifest_full"
	}
}

func restoreSourceSummary(source executeSource) string {
	if source.SourceKind == "manifest" {
		return "Restore source resolution completed from manifest"
	}

	switch source.SelectionMode {
	case "direct_db_only":
		return "Restore source resolution completed from a direct database backup"
	case "direct_files_only":
		return "Restore source resolution completed from a direct files backup"
	default:
		return "Restore source resolution completed from a direct backup pair"
	}
}

func restoreSourceDetails(source executeSource) string {
	switch source.SourceKind {
	case "manifest":
		return fmt.Sprintf("Using manifest %s with database backup %s and files backup %s.", source.ManifestJSON, source.DBBackup, source.FilesBackup)
	case "direct_db_only":
		return fmt.Sprintf("Using direct database backup %s.", source.DBBackup)
	case "direct_files_only":
		return fmt.Sprintf("Using direct files backup %s.", source.FilesBackup)
	default:
		return fmt.Sprintf("Using direct database backup %s and files backup %s.", source.DBBackup, source.FilesBackup)
	}
}
