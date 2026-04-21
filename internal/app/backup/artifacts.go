package backup

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	domainbackup "github.com/lazuale/espocrm-ops/internal/domain/backup"
	domainfailure "github.com/lazuale/espocrm-ops/internal/domain/failure"
)

type backupExecutionState struct {
	createdAt              time.Time
	set                    domainbackup.BackupSet
	appServicesWereRunning bool
	dbChecksum             string
	filesChecksum          string
	dbSizeBytes            int64
	filesSizeBytes         int64
}

type backupManifestJSON struct {
	Version                int                     `json:"version"`
	Scope                  string                  `json:"scope"`
	Contour                string                  `json:"contour"`
	CreatedAt              string                  `json:"created_at"`
	ComposeProject         string                  `json:"compose_project"`
	Artifacts              backupManifestArtifacts `json:"artifacts"`
	Checksums              backupManifestChecksums `json:"checksums"`
	EnvFile                string                  `json:"env_file"`
	EspoCRMImage           string                  `json:"espocrm_image"`
	MariaDBTag             string                  `json:"mariadb_tag"`
	RetentionDays          int                     `json:"retention_days"`
	ConsistentSnapshot     bool                    `json:"consistent_snapshot"`
	AppServicesWereRunning bool                    `json:"app_services_were_running"`
	DBBackupCreated        bool                    `json:"db_backup_created"`
	FilesBackupCreated     bool                    `json:"files_backup_created"`
}

type backupManifestArtifacts struct {
	DBBackup    string `json:"db_backup"`
	FilesBackup string `json:"files_backup"`
}

type backupManifestChecksums struct {
	DBBackup    string `json:"db_backup"`
	FilesBackup string `json:"files_backup"`
}

type filesArchiveInfo struct {
	UsedDockerHelper bool
}

func allocateBackupExecutionState(req PreparedRequest) (backupExecutionState, error) {
	createdAt := executeNow(req.Now)
	for {
		stamp := createdAt.Format("2006-01-02_15-04-05")
		set := domainbackup.BuildBackupSet(req.BackupRoot, req.NamePrefix, stamp)
		for _, dir := range []string{
			filepath.Dir(set.DBBackup.Path),
			filepath.Dir(set.FilesBackup.Path),
			filepath.Dir(set.ManifestTXT.Path),
			filepath.Dir(set.ManifestJSON.Path),
		} {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return backupExecutionState{}, fmt.Errorf("ensure backup directory %s: %w", dir, err)
			}
		}
		if !backupSetCollides(set) {
			return backupExecutionState{
				createdAt: createdAt,
				set:       set,
			}, nil
		}

		createdAt = createdAt.Add(time.Second)
	}
}

func backupTempPaths(state backupExecutionState, req PreparedRequest) []string {
	paths := []string{
		state.set.ManifestTXT.Path + ".tmp",
		state.set.ManifestJSON.Path + ".tmp",
	}
	if !req.SkipDB {
		paths = append(paths, state.set.DBBackup.Path+".tmp", state.set.DBBackup.Path+".sha256.tmp")
	}
	if !req.SkipFiles {
		paths = append(paths, state.set.FilesBackup.Path+".tmp", state.set.FilesBackup.Path+".sha256.tmp")
	}

	return paths
}

func backupSetCollides(set domainbackup.BackupSet) bool {
	paths := []string{
		set.DBBackup.Path,
		set.FilesBackup.Path,
		set.ManifestTXT.Path,
		set.ManifestJSON.Path,
	}

	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}

	return false
}

func (s Service) createFilesBackupArchive(req PreparedRequest, archivePath string) (filesArchiveInfo, error) {
	info := filesArchiveInfo{}
	if err := s.files.CreateTarGz(req.StorageDir, archivePath); err == nil {
		return info, nil
	}

	info.UsedDockerHelper = true
	if err := s.runtime.CreateTarArchiveViaHelper(req.StorageDir, archivePath, req.MariaDBTag, req.EspoCRMImage); err != nil {
		return filesArchiveInfo{}, domainfailure.Failure{
			Kind:    domainfailure.KindExternal,
			Code:    "backup_failed",
			Summary: "Files backup failed",
			Action:  "Ensure the storage directory is readable and the Docker helper can archive it before rerunning backup.",
			Err:     fmt.Errorf("could not archive application files %s: %w", req.StorageDir, err),
		}
	}

	return info, nil
}

func (s Service) finalizeBackupArtifacts(req PreparedRequest, state *backupExecutionState, info *ExecuteInfo) error {
	manifestTXTPath := state.set.ManifestTXT.Path
	manifestJSONPath := state.set.ManifestJSON.Path
	manifestTXTTmp := manifestTXTPath + ".tmp"
	manifestJSONTmp := manifestJSONPath + ".tmp"

	if !req.SkipDB {
		dbChecksum, err := s.files.SHA256File(state.set.DBBackup.Path)
		if err != nil {
			return fmt.Errorf("hash db backup: %w", err)
		}
		state.dbChecksum = dbChecksum
		dbInfo, err := os.Stat(state.set.DBBackup.Path)
		if err != nil {
			return fmt.Errorf("stat db backup: %w", err)
		}
		state.dbSizeBytes = dbInfo.Size()

		if err := s.store.WriteSHA256Sidecar(state.set.DBBackup.Path, state.dbChecksum, info.DBSidecarPath+".tmp"); err != nil {
			return fmt.Errorf("write db checksum sidecar: %w", err)
		}
		if err := saveTempFile(info.DBSidecarPath+".tmp", info.DBSidecarPath, "save db checksum sidecar"); err != nil {
			return err
		}
	}

	if !req.SkipFiles {
		filesChecksum, err := s.files.SHA256File(state.set.FilesBackup.Path)
		if err != nil {
			return fmt.Errorf("hash files backup: %w", err)
		}
		state.filesChecksum = filesChecksum
		filesInfo, err := os.Stat(state.set.FilesBackup.Path)
		if err != nil {
			return fmt.Errorf("stat files backup: %w", err)
		}
		state.filesSizeBytes = filesInfo.Size()

		if err := s.store.WriteSHA256Sidecar(state.set.FilesBackup.Path, state.filesChecksum, info.FilesSidecarPath+".tmp"); err != nil {
			return fmt.Errorf("write files checksum sidecar: %w", err)
		}
		if err := saveTempFile(info.FilesSidecarPath+".tmp", info.FilesSidecarPath, "save files checksum sidecar"); err != nil {
			return err
		}
	}

	if err := s.writeBackupManifestTXT(req, *state, manifestTXTTmp); err != nil {
		return err
	}
	if err := saveTempFile(manifestTXTTmp, manifestTXTPath, "save text manifest"); err != nil {
		return err
	}

	if err := s.writeBackupManifestJSON(req, *state, manifestJSONTmp); err != nil {
		return err
	}
	if err := saveTempFile(manifestJSONTmp, manifestJSONPath, "save json manifest"); err != nil {
		return err
	}

	return nil
}

func (s Service) writeBackupManifestTXT(req PreparedRequest, state backupExecutionState, path string) error {
	var body strings.Builder
	stamp := state.createdAt.UTC().Format("2006-01-02_15-04-05")

	fmt.Fprintf(&body, "created_at=%s\n", stamp)
	fmt.Fprintf(&body, "contour=%s\n", req.Scope)
	fmt.Fprintf(&body, "compose_project=%s\n", req.ComposeProject)
	fmt.Fprintf(&body, "env_file=%s\n", filepath.Base(req.EnvFile))
	fmt.Fprintf(&body, "espocrm_image=%s\n", req.EspoCRMImage)
	fmt.Fprintf(&body, "mariadb_tag=%s\n", req.MariaDBTag)
	fmt.Fprintf(&body, "retention_days=%d\n", req.RetentionDays)
	fmt.Fprintf(&body, "db_backup_created=%d\n", boolAsInt(!req.SkipDB))
	fmt.Fprintf(&body, "files_backup_created=%d\n", boolAsInt(!req.SkipFiles))
	fmt.Fprintf(&body, "consistent_snapshot=%d\n", boolAsInt(!req.NoStop))
	fmt.Fprintf(&body, "app_services_were_running=%d\n", boolAsInt(state.appServicesWereRunning))

	if !req.SkipDB {
		fmt.Fprintf(&body, "db_backup_file=%s\n", filepath.Base(state.set.DBBackup.Path))
		fmt.Fprintf(&body, "db_backup_checksum_file=%s\n", filepath.Base(state.set.DBBackup.Path)+".sha256")
		fmt.Fprintf(&body, "db_backup_sha256=%s\n", state.dbChecksum)
		fmt.Fprintf(&body, "db_backup_size_bytes=%d\n", state.dbSizeBytes)
	}
	if !req.SkipFiles {
		fmt.Fprintf(&body, "files_backup_file=%s\n", filepath.Base(state.set.FilesBackup.Path))
		fmt.Fprintf(&body, "files_backup_checksum_file=%s\n", filepath.Base(state.set.FilesBackup.Path)+".sha256")
		fmt.Fprintf(&body, "files_backup_sha256=%s\n", state.filesChecksum)
		fmt.Fprintf(&body, "files_backup_size_bytes=%d\n", state.filesSizeBytes)
	}

	if err := os.WriteFile(path, []byte(body.String()), 0o644); err != nil {
		return fmt.Errorf("write text manifest: %w", err)
	}

	return nil
}

func (s Service) writeBackupManifestJSON(req PreparedRequest, state backupExecutionState, path string) error {
	manifest := backupManifestJSON{
		Version:        1,
		Scope:          req.Scope,
		Contour:        req.Scope,
		CreatedAt:      state.createdAt.UTC().Format(time.RFC3339),
		ComposeProject: req.ComposeProject,
		Artifacts: backupManifestArtifacts{
			DBBackup:    maybeBaseName(!req.SkipDB, state.set.DBBackup.Path),
			FilesBackup: maybeBaseName(!req.SkipFiles, state.set.FilesBackup.Path),
		},
		Checksums: backupManifestChecksums{
			DBBackup:    maybeString(!req.SkipDB, state.dbChecksum),
			FilesBackup: maybeString(!req.SkipFiles, state.filesChecksum),
		},
		EnvFile:                filepath.Base(req.EnvFile),
		EspoCRMImage:           req.EspoCRMImage,
		MariaDBTag:             req.MariaDBTag,
		RetentionDays:          req.RetentionDays,
		ConsistentSnapshot:     !req.NoStop,
		AppServicesWereRunning: state.appServicesWereRunning,
		DBBackupCreated:        !req.SkipDB,
		FilesBackupCreated:     !req.SkipFiles,
	}

	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal json manifest: %w", err)
	}
	raw = append(raw, '\n')

	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return fmt.Errorf("write json manifest: %w", err)
	}

	return nil
}

func cleanupBackupRetention(root string, retentionDays int, now time.Time) error {
	if retentionDays < 0 {
		return fmt.Errorf("retention days must be non-negative")
	}

	cutoff := now.Add(-time.Duration(retentionDays+1) * 24 * time.Hour)
	targets := []struct {
		dir      string
		patterns []string
	}{
		{dir: filepath.Join(root, "db"), patterns: []string{"*.sql.gz", "*.sql.gz.sha256"}},
		{dir: filepath.Join(root, "files"), patterns: []string{"*.tar.gz", "*.tar.gz.sha256"}},
		{dir: filepath.Join(root, "manifests"), patterns: []string{"*.manifest.txt", "*.manifest.json"}},
	}

	for _, target := range targets {
		for _, pattern := range target.patterns {
			matches, err := filepath.Glob(filepath.Join(target.dir, pattern))
			if err != nil {
				return fmt.Errorf("cleanup retention glob %s: %w", filepath.Join(target.dir, pattern), err)
			}
			for _, match := range matches {
				info, err := os.Stat(match)
				if err != nil {
					if os.IsNotExist(err) {
						continue
					}
					return fmt.Errorf("stat retention candidate %s: %w", match, err)
				}
				if info.IsDir() || !info.ModTime().Before(cutoff) {
					continue
				}
				if err := os.Remove(match); err != nil && !os.IsNotExist(err) {
					return fmt.Errorf("remove retention candidate %s: %w", match, err)
				}
			}
		}
	}

	return nil
}

func cleanupBackupTemps(paths []string) {
	for _, path := range paths {
		if strings.TrimSpace(path) == "" {
			continue
		}
		_ = os.Remove(path)
	}
}

func saveTempFile(tmpPath, finalPath, action string) error {
	if err := os.Rename(tmpPath, finalPath); err != nil {
		return fmt.Errorf("%s: %w", action, err)
	}

	return nil
}

func boolAsInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func maybeBaseName(include bool, path string) string {
	if !include {
		return ""
	}
	return filepath.Base(path)
}

func maybeString(include bool, value string) string {
	if !include {
		return ""
	}
	return value
}
