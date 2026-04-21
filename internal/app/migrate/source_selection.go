package migrate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	backupstoreport "github.com/lazuale/espocrm-ops/internal/app/ports/backupstoreport"
	domainbackup "github.com/lazuale/espocrm-ops/internal/domain/backup"
	domainenv "github.com/lazuale/espocrm-ops/internal/domain/env"
)

func (s Service) resolveSourceSelection(env domainenv.OperationEnv, req ExecuteRequest) (sourceSelection, error) {
	backupRoot := s.env.ResolveProjectPath(filepath.Clean(req.ProjectDir), env.BackupRoot())
	dbBackup := strings.TrimSpace(req.DBBackup)
	filesBackup := strings.TrimSpace(req.FilesBackup)

	switch {
	case req.SkipDB:
		return s.resolveFilesOnlySelection(backupRoot, filesBackup)
	case req.SkipFiles:
		return s.resolveDBOnlySelection(backupRoot, dbBackup)
	case dbBackup == "" && filesBackup == "":
		return s.resolveLatestCompleteSelection(backupRoot)
	case dbBackup != "" && filesBackup != "":
		return s.resolveFullPairSelection(backupRoot, dbBackup, filesBackup, "explicit_pair")
	case dbBackup != "":
		group, err := domainbackup.ParseDBBackupName(dbBackup)
		if err != nil {
			return sourceSelection{}, executeFailure{
				Summary: "The explicit database backup name is unsupported",
				Action:  "Pass a canonical .sql.gz database backup path or provide a full explicit pair.",
				Err:     err,
			}
		}
		set := domainbackup.BuildBackupSet(backupRoot, group.Prefix, group.Stamp)
		return s.resolveFullPairSelection(backupRoot, dbBackup, set.FilesBackup.Path, "paired_from_db")
	default:
		group, err := domainbackup.ParseFilesBackupName(filesBackup)
		if err != nil {
			return sourceSelection{}, executeFailure{
				Summary: "The explicit files backup name is unsupported",
				Action:  "Pass a canonical .tar.gz files backup path or provide a full explicit pair.",
				Err:     err,
			}
		}
		set := domainbackup.BuildBackupSet(backupRoot, group.Prefix, group.Stamp)
		return s.resolveFullPairSelection(backupRoot, set.DBBackup.Path, filesBackup, "paired_from_files")
	}
}

func (s Service) resolveLatestCompleteSelection(backupRoot string) (sourceSelection, error) {
	groups, err := s.store.Groups(backupRoot, backupstoreport.GroupModeDB)
	if err != nil {
		return sourceSelection{}, executeFailure{
			Summary: "Automatic source backup selection could not inspect the source backup root",
			Action:  "Check the source BACKUP_ROOT and rerun migrate.",
			Err:     err,
		}
	}

	for _, group := range groups {
		set := domainbackup.BuildBackupSet(backupRoot, group.Prefix, group.Stamp)
		if err := s.store.VerifyDirectDBBackup(set.DBBackup.Path); err != nil {
			continue
		}
		if err := s.store.VerifyDirectFilesBackup(set.FilesBackup.Path); err != nil {
			continue
		}

		selection := sourceSelection{
			SelectionMode: "auto_latest_complete",
			Prefix:        group.Prefix,
			Stamp:         group.Stamp,
			DBBackup:      set.DBBackup.Path,
			FilesBackup:   set.FilesBackup.Path,
		}
		if err := s.attachMatchingManifest(backupRoot, &selection); err != nil {
			return sourceSelection{}, err
		}
		return selection, nil
	}

	return sourceSelection{}, executeFailure{
		Summary: "Automatic source backup selection could not find a complete verified backup set",
		Action:  "Create or repair a coherent source backup set, or pass explicit backup paths.",
		Err:     fmt.Errorf("no complete verified backup pair found under %s", backupRoot),
	}
}

func (s Service) resolveFullPairSelection(backupRoot, dbPath, filesPath, mode string) (sourceSelection, error) {
	dbPath = filepath.Clean(dbPath)
	filesPath = filepath.Clean(filesPath)

	if err := s.store.VerifyDirectDBBackup(dbPath); err != nil {
		return sourceSelection{}, executeFailure{
			Summary: "The selected database backup is not valid",
			Action:  "Choose a readable .sql.gz backup with a valid .sha256 sidecar.",
			Err:     err,
		}
	}
	if err := s.store.VerifyDirectFilesBackup(filesPath); err != nil {
		return sourceSelection{}, executeFailure{
			Summary: "The selected files backup is not valid",
			Action:  "Choose a readable .tar.gz backup with a valid .sha256 sidecar.",
			Err:     err,
		}
	}

	dbGroup, err := domainbackup.ParseDBBackupName(dbPath)
	if err != nil {
		return sourceSelection{}, executeFailure{
			Summary: "The selected database backup name is unsupported",
			Action:  "Rename the database backup to the canonical pattern or choose a supported backup set.",
			Err:     err,
		}
	}
	filesGroup, err := domainbackup.ParseFilesBackupName(filesPath)
	if err != nil {
		return sourceSelection{}, executeFailure{
			Summary: "The selected files backup name is unsupported",
			Action:  "Rename the files backup to the canonical pattern or choose a supported backup set.",
			Err:     err,
		}
	}
	if dbGroup != filesGroup {
		return sourceSelection{}, executeFailure{
			Summary: "The selected database and files backups do not belong to the same backup set",
			Action:  "Pass a DB backup and files backup from the same backup set.",
			Err:     fmt.Errorf("database backup resolves to %s %s, but files backup resolves to %s %s", dbGroup.Prefix, dbGroup.Stamp, filesGroup.Prefix, filesGroup.Stamp),
		}
	}

	selection := sourceSelection{
		SelectionMode: mode,
		Prefix:        dbGroup.Prefix,
		Stamp:         dbGroup.Stamp,
		DBBackup:      dbPath,
		FilesBackup:   filesPath,
	}
	if err := s.attachMatchingManifest(backupRoot, &selection); err != nil {
		return sourceSelection{}, err
	}
	return selection, nil
}

func (s Service) resolveDBOnlySelection(backupRoot, explicitDB string) (sourceSelection, error) {
	if explicitDB != "" {
		dbPath := filepath.Clean(explicitDB)
		if err := s.store.VerifyDirectDBBackup(dbPath); err != nil {
			return sourceSelection{}, executeFailure{
				Summary: "The selected database backup is not valid",
				Action:  "Choose a readable .sql.gz backup with a valid .sha256 sidecar.",
				Err:     err,
			}
		}
		group, err := domainbackup.ParseDBBackupName(dbPath)
		if err != nil {
			return sourceSelection{}, executeFailure{
				Summary: "The selected database backup name is unsupported",
				Action:  "Rename the database backup to the canonical pattern or choose a supported backup set.",
				Err:     err,
			}
		}
		return sourceSelection{
			SelectionMode: "explicit_db_only",
			Prefix:        group.Prefix,
			Stamp:         group.Stamp,
			DBBackup:      dbPath,
		}, nil
	}

	groups, err := s.store.Groups(backupRoot, backupstoreport.GroupModeDB)
	if err != nil {
		return sourceSelection{}, executeFailure{
			Summary: "Automatic database backup selection could not inspect the source backup root",
			Action:  "Check the source BACKUP_ROOT and rerun migrate.",
			Err:     err,
		}
	}

	for _, group := range groups {
		set := domainbackup.BuildBackupSet(backupRoot, group.Prefix, group.Stamp)
		if err := s.store.VerifyDirectDBBackup(set.DBBackup.Path); err != nil {
			continue
		}
		return sourceSelection{
			SelectionMode: "auto_latest_db",
			Prefix:        group.Prefix,
			Stamp:         group.Stamp,
			DBBackup:      set.DBBackup.Path,
		}, nil
	}

	return sourceSelection{}, executeFailure{
		Summary: "Automatic database backup selection could not find a verified database backup",
		Action:  "Create or repair a source database backup, or pass --db-backup explicitly.",
		Err:     fmt.Errorf("no verified database backup found under %s", filepath.Join(backupRoot, "db")),
	}
}

func (s Service) resolveFilesOnlySelection(backupRoot, explicitFiles string) (sourceSelection, error) {
	if explicitFiles != "" {
		filesPath := filepath.Clean(explicitFiles)
		if err := s.store.VerifyDirectFilesBackup(filesPath); err != nil {
			return sourceSelection{}, executeFailure{
				Summary: "The selected files backup is not valid",
				Action:  "Choose a readable .tar.gz backup with a valid .sha256 sidecar.",
				Err:     err,
			}
		}
		group, err := domainbackup.ParseFilesBackupName(filesPath)
		if err != nil {
			return sourceSelection{}, executeFailure{
				Summary: "The selected files backup name is unsupported",
				Action:  "Rename the files backup to the canonical pattern or choose a supported backup set.",
				Err:     err,
			}
		}
		return sourceSelection{
			SelectionMode: "explicit_files_only",
			Prefix:        group.Prefix,
			Stamp:         group.Stamp,
			FilesBackup:   filesPath,
		}, nil
	}

	groups, err := s.store.Groups(backupRoot, backupstoreport.GroupModeFiles)
	if err != nil {
		return sourceSelection{}, executeFailure{
			Summary: "Automatic files backup selection could not inspect the source backup root",
			Action:  "Check the source BACKUP_ROOT and rerun migrate.",
			Err:     err,
		}
	}

	for _, group := range groups {
		set := domainbackup.BuildBackupSet(backupRoot, group.Prefix, group.Stamp)
		if err := s.store.VerifyDirectFilesBackup(set.FilesBackup.Path); err != nil {
			continue
		}
		return sourceSelection{
			SelectionMode: "auto_latest_files",
			Prefix:        group.Prefix,
			Stamp:         group.Stamp,
			FilesBackup:   set.FilesBackup.Path,
		}, nil
	}

	return sourceSelection{}, executeFailure{
		Summary: "Automatic files backup selection could not find a verified files backup",
		Action:  "Create or repair a source files backup, or pass --files-backup explicitly.",
		Err:     fmt.Errorf("no verified files backup found under %s", filepath.Join(backupRoot, "files")),
	}
}

func (s Service) attachMatchingManifest(backupRoot string, selection *sourceSelection) error {
	if selection == nil || strings.TrimSpace(selection.DBBackup) == "" || strings.TrimSpace(selection.FilesBackup) == "" {
		return nil
	}

	set := domainbackup.BuildBackupSet(backupRoot, selection.Prefix, selection.Stamp)
	if filepath.Clean(selection.DBBackup) != filepath.Clean(set.DBBackup.Path) || filepath.Clean(selection.FilesBackup) != filepath.Clean(set.FilesBackup.Path) {
		return nil
	}

	if _, err := os.Stat(set.ManifestJSON.Path); err != nil {
		return nil
	}

	info, err := s.store.VerifyManifestDetailed(set.ManifestJSON.Path)
	if err != nil {
		return executeFailure{
			Summary: "The matching manifest for the selected backup set is not valid",
			Action:  "Repair or remove the invalid manifest under BACKUP_ROOT before rerunning migrate.",
			Err:     err,
		}
	}

	selection.ManifestJSON = info.ManifestPath
	if _, err := os.Stat(set.ManifestTXT.Path); err == nil {
		selection.ManifestTXT = set.ManifestTXT.Path
	}

	return nil
}
