package backup

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/lazuale/espocrm-ops/internal/contract/apperr"
	domainbackup "github.com/lazuale/espocrm-ops/internal/domain/backup"
	platformclock "github.com/lazuale/espocrm-ops/internal/platform/clock"
	platformdocker "github.com/lazuale/espocrm-ops/internal/platform/docker"
	platformfs "github.com/lazuale/espocrm-ops/internal/platform/fs"
)

var backupAppServices = []string{
	"espocrm",
	"espocrm-daemon",
	"espocrm-websocket",
}

type ExecuteRequest struct {
	Scope          string
	ProjectDir     string
	ComposeFile    string
	EnvFile        string
	BackupRoot     string
	StorageDir     string
	NamePrefix     string
	RetentionDays  int
	ComposeProject string
	DBUser         string
	DBPassword     string
	DBName         string
	EspoCRMImage   string
	MariaDBTag     string
	SkipDB         bool
	SkipFiles      bool
	NoStop         bool
	LogWriter      io.Writer
	ErrWriter      io.Writer
}

type ExecuteInfo struct {
	Scope                  string
	CreatedAt              string
	ConsistentSnapshot     bool
	AppServicesWereRunning bool
	DBBackupCreated        bool
	FilesBackupCreated     bool
	ManifestTXTPath        string
	ManifestJSONPath       string
	DBBackupPath           string
	FilesBackupPath        string
	DBSidecarPath          string
	FilesSidecarPath       string
}

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

func ExecuteBackup(req ExecuteRequest) (info ExecuteInfo, err error) {
	info = ExecuteInfo{
		Scope:              req.Scope,
		ConsistentSnapshot: !req.NoStop,
		DBBackupCreated:    !req.SkipDB,
		FilesBackupCreated: !req.SkipFiles,
	}

	if req.SkipDB && req.SkipFiles {
		return info, apperr.Wrap(apperr.KindValidation, "backup_failed", fmt.Errorf("nothing to back up: --skip-db and --skip-files cannot both be set"))
	}

	state, err := allocateBackupExecutionState(req, req.ErrWriter)
	if err != nil {
		return info, wrapBackupIO(err)
	}

	info.CreatedAt = state.createdAt.UTC().Format(time.RFC3339)
	info.ManifestTXTPath = state.set.ManifestTXT.Path
	info.ManifestJSONPath = state.set.ManifestJSON.Path
	if !req.SkipDB {
		info.DBBackupPath = state.set.DBBackup.Path
		info.DBSidecarPath = state.set.DBBackup.Path + ".sha256"
	}
	if !req.SkipFiles {
		info.FilesBackupPath = state.set.FilesBackup.Path
		info.FilesSidecarPath = state.set.FilesBackup.Path + ".sha256"
	}

	tmpPaths := []string{
		state.set.ManifestTXT.Path + ".tmp",
		state.set.ManifestJSON.Path + ".tmp",
	}
	if !req.SkipDB {
		tmpPaths = append(tmpPaths, state.set.DBBackup.Path+".tmp", state.set.DBBackup.Path+".sha256.tmp")
	}
	if !req.SkipFiles {
		tmpPaths = append(tmpPaths, state.set.FilesBackup.Path+".tmp", state.set.FilesBackup.Path+".sha256.tmp")
	}

	cfg := platformdocker.ComposeConfig{
		ProjectDir:  req.ProjectDir,
		ComposeFile: req.ComposeFile,
		EnvFile:     req.EnvFile,
	}

	restartAppServicesOnFailure := false
	defer cleanupBackupTemps(tmpPaths)
	defer func() {
		if !restartAppServicesOnFailure || err == nil {
			return
		}

		if warnErr := warnBackup(req.ErrWriter, "Backup failed unexpectedly, restoring application services"); warnErr != nil {
			err = errors.Join(err, wrapBackupIO(warnErr))
		}
		if startErr := platformdocker.ComposeUp(cfg, backupAppServices...); startErr != nil {
			if warnErr := warnBackup(req.ErrWriter, "Could not automatically restart application services after backup failure"); warnErr != nil {
				err = errors.Join(err, wrapBackupIO(warnErr))
			}
			err = fmt.Errorf("%w; automatic application service restart failed: %v", err, startErr)
			return
		}
		restartAppServicesOnFailure = false
	}()

	if !req.NoStop {
		runningServices, err := platformdocker.ComposeRunningServices(cfg)
		if err != nil {
			return info, wrapBackupExternal(err)
		}
		if backupAppServicesRunning(runningServices) {
			state.appServicesWereRunning = true
			info.AppServicesWereRunning = true
			restartAppServicesOnFailure = true
			if err := logBackup(req.LogWriter, "Stopping application services for a consistent backup"); err != nil {
				return info, wrapBackupIO(err)
			}
			if err := platformdocker.ComposeStop(cfg, backupAppServices...); err != nil {
				return info, wrapBackupExternal(err)
			}
		} else {
			if err := logBackup(req.LogWriter, "Application services are already stopped, no extra stop is required"); err != nil {
				return info, wrapBackupIO(err)
			}
		}
	} else {
		if err := warnBackup(req.ErrWriter, "Backup is being created without stopping application services: strict consistency is not guaranteed"); err != nil {
			return info, wrapBackupIO(err)
		}
	}

	if !req.SkipDB {
		if err := logBackup(req.LogWriter, "[1/4] Creating database dump: %s", state.set.DBBackup.Path); err != nil {
			return info, wrapBackupIO(err)
		}
		if err := platformdocker.DumpMySQLDumpGz(cfg, "db", req.DBUser, req.DBPassword, req.DBName, state.set.DBBackup.Path+".tmp"); err != nil {
			return info, wrapBackupExternal(err)
		}
		if err := os.Rename(state.set.DBBackup.Path+".tmp", state.set.DBBackup.Path); err != nil {
			return info, wrapBackupIO(fmt.Errorf("save db backup: %w", err))
		}
	} else {
		if err := logBackup(req.LogWriter, "[1/4] Database backup skipped because of --skip-db"); err != nil {
			return info, wrapBackupIO(err)
		}
	}

	if !req.SkipFiles {
		if err := logBackup(req.LogWriter, "[2/4] Archiving application files: %s", state.set.FilesBackup.Path); err != nil {
			return info, wrapBackupIO(err)
		}
		if err := createFilesBackupArchive(req, state.set.FilesBackup.Path+".tmp"); err != nil {
			return info, err
		}
		if err := os.Rename(state.set.FilesBackup.Path+".tmp", state.set.FilesBackup.Path); err != nil {
			return info, wrapBackupIO(fmt.Errorf("save files backup: %w", err))
		}
	} else {
		if err := logBackup(req.LogWriter, "[2/4] Files backup skipped because of --skip-files"); err != nil {
			return info, wrapBackupIO(err)
		}
	}

	if err := logBackup(req.LogWriter, "[3/4] Generating checksums and manifest files"); err != nil {
		return info, wrapBackupIO(err)
	}
	if err := finalizeBackupArtifacts(req, &state, &info); err != nil {
		return info, err
	}

	if err := logBackup(req.LogWriter, "[4/4] Removing backups older than %d days", req.RetentionDays); err != nil {
		return info, wrapBackupIO(err)
	}
	if err := cleanupBackupRetention(req.BackupRoot, req.RetentionDays); err != nil {
		return info, wrapBackupIO(err)
	}

	if info.AppServicesWereRunning {
		if err := logBackup(req.LogWriter, "Starting application services after a successful backup"); err != nil {
			return info, wrapBackupIO(err)
		}
		if err := platformdocker.ComposeUp(cfg, backupAppServices...); err != nil {
			return info, wrapBackupExternal(err)
		}
		restartAppServicesOnFailure = false
	}

	if err := logBackup(req.LogWriter, "Backup completed:"); err != nil {
		return info, wrapBackupIO(err)
	}
	if !req.SkipDB {
		if err := logBackup(req.LogWriter, "  Database: %s", info.DBBackupPath); err != nil {
			return info, wrapBackupIO(err)
		}
		if err := logBackup(req.LogWriter, "  Database checksum: %s", info.DBSidecarPath); err != nil {
			return info, wrapBackupIO(err)
		}
	}
	if !req.SkipFiles {
		if err := logBackup(req.LogWriter, "  Files:       %s", info.FilesBackupPath); err != nil {
			return info, wrapBackupIO(err)
		}
		if err := logBackup(req.LogWriter, "  Files checksum: %s", info.FilesSidecarPath); err != nil {
			return info, wrapBackupIO(err)
		}
	}
	if err := logBackup(req.LogWriter, "  Text manifest: %s", info.ManifestTXTPath); err != nil {
		return info, wrapBackupIO(err)
	}
	if err := logBackup(req.LogWriter, "  JSON manifest:      %s", info.ManifestJSONPath); err != nil {
		return info, wrapBackupIO(err)
	}

	return info, nil
}

func allocateBackupExecutionState(req ExecuteRequest, errWriter io.Writer) (backupExecutionState, error) {
	createdAt := platformclock.Now().UTC()
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

		if err := warnBackup(errWriter, "Detected a name collision on second-level stamp '%s', waiting for the next free stamp", stamp); err != nil {
			return backupExecutionState{}, err
		}
		createdAt = createdAt.Add(time.Second)
	}
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

func backupAppServicesRunning(runningServices []string) bool {
	for _, service := range runningServices {
		if slices.Contains(backupAppServices, service) {
			return true
		}
	}

	return false
}

func createFilesBackupArchive(req ExecuteRequest, archivePath string) error {
	if err := platformfs.CreateTarGz(req.StorageDir, archivePath); err == nil {
		return nil
	}

	if err := warnBackup(req.ErrWriter, "Local archiving failed, trying Docker fallback: %s", req.StorageDir); err != nil {
		return wrapBackupIO(err)
	}
	if err := platformdocker.CreateTarArchiveViaHelper(req.StorageDir, archivePath, req.MariaDBTag, req.EspoCRMImage); err != nil {
		return wrapBackupExternal(fmt.Errorf("could not archive application files: %s: %w", req.StorageDir, err))
	}

	return nil
}

func finalizeBackupArtifacts(req ExecuteRequest, state *backupExecutionState, info *ExecuteInfo) error {
	manifestTXTPath := state.set.ManifestTXT.Path
	manifestJSONPath := state.set.ManifestJSON.Path
	manifestTXTTmp := manifestTXTPath + ".tmp"
	manifestJSONTmp := manifestJSONPath + ".tmp"

	if !req.SkipDB {
		dbChecksum, err := platformfs.SHA256File(state.set.DBBackup.Path)
		if err != nil {
			return wrapBackupIO(fmt.Errorf("hash db backup: %w", err))
		}
		state.dbChecksum = dbChecksum
		dbInfo, err := os.Stat(state.set.DBBackup.Path)
		if err != nil {
			return wrapBackupIO(fmt.Errorf("stat db backup: %w", err))
		}
		state.dbSizeBytes = dbInfo.Size()

		if err := WriteSHA256Sidecar(state.set.DBBackup.Path, state.dbChecksum, info.DBSidecarPath+".tmp"); err != nil {
			return wrapBackupIO(fmt.Errorf("write db checksum sidecar: %w", err))
		}
		if err := os.Rename(info.DBSidecarPath+".tmp", info.DBSidecarPath); err != nil {
			return wrapBackupIO(fmt.Errorf("save db checksum sidecar: %w", err))
		}
	}

	if !req.SkipFiles {
		filesChecksum, err := platformfs.SHA256File(state.set.FilesBackup.Path)
		if err != nil {
			return wrapBackupIO(fmt.Errorf("hash files backup: %w", err))
		}
		state.filesChecksum = filesChecksum
		filesInfo, err := os.Stat(state.set.FilesBackup.Path)
		if err != nil {
			return wrapBackupIO(fmt.Errorf("stat files backup: %w", err))
		}
		state.filesSizeBytes = filesInfo.Size()

		if err := WriteSHA256Sidecar(state.set.FilesBackup.Path, state.filesChecksum, info.FilesSidecarPath+".tmp"); err != nil {
			return wrapBackupIO(fmt.Errorf("write files checksum sidecar: %w", err))
		}
		if err := os.Rename(info.FilesSidecarPath+".tmp", info.FilesSidecarPath); err != nil {
			return wrapBackupIO(fmt.Errorf("save files checksum sidecar: %w", err))
		}
	}

	if err := writeBackupManifestTXT(req, *state, manifestTXTTmp); err != nil {
		return wrapBackupIO(err)
	}
	if err := os.Rename(manifestTXTTmp, manifestTXTPath); err != nil {
		return wrapBackupIO(fmt.Errorf("save text manifest: %w", err))
	}

	if err := writeBackupManifestJSON(req, *state, manifestJSONTmp); err != nil {
		return wrapBackupIO(err)
	}
	if err := os.Rename(manifestJSONTmp, manifestJSONPath); err != nil {
		return wrapBackupIO(fmt.Errorf("save json manifest: %w", err))
	}

	return nil
}

func writeBackupManifestTXT(req ExecuteRequest, state backupExecutionState, path string) error {
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

func writeBackupManifestJSON(req ExecuteRequest, state backupExecutionState, path string) error {
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

func cleanupBackupRetention(root string, retentionDays int) error {
	if retentionDays < 0 {
		return fmt.Errorf("retention days must be non-negative")
	}

	cutoff := platformclock.Now().UTC().Add(-time.Duration(retentionDays+1) * 24 * time.Hour)
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

func logBackup(w io.Writer, format string, args ...any) error {
	if w == nil {
		return nil
	}
	_, err := fmt.Fprintf(w, format+"\n", args...)
	return err
}

func warnBackup(w io.Writer, format string, args ...any) error {
	if w == nil {
		return nil
	}
	_, err := fmt.Fprintf(w, "[warn] "+format+"\n", args...)
	return err
}

func wrapBackupIO(err error) error {
	return apperr.Wrap(apperr.KindIO, "backup_failed", err)
}

func wrapBackupExternal(err error) error {
	return apperr.Wrap(apperr.KindExternal, "backup_failed", err)
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
