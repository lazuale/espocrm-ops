package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lazuale/espocrm-ops/internal/model"
)

type FileStore struct{}

func (FileStore) EnsureLayout(ctx context.Context, layout model.BackupLayout) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, dir := range []string{
		filepath.Join(layout.Root, "db"),
		filepath.Join(layout.Root, "files"),
		filepath.Join(layout.Root, "manifests"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("создать каталог backup %s: %w", dir, err)
		}
	}
	return nil
}

func (FileStore) AnyExists(ctx context.Context, paths []string) (bool, error) {
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		if strings.TrimSpace(path) == "" {
			continue
		}
		if _, err := os.Stat(path); err == nil {
			return true, nil
		} else if !os.IsNotExist(err) {
			return false, fmt.Errorf("проверить путь %s: %w", path, err)
		}
	}
	return false, nil
}

func (FileStore) WriteArtifact(ctx context.Context, path string, body io.Reader) (model.Artifact, error) {
	if err := ctx.Err(); err != nil {
		return model.Artifact{}, err
	}
	if strings.TrimSpace(path) == "" {
		return model.Artifact{}, fmt.Errorf("путь artifact обязателен")
	}
	if _, err := os.Stat(path); err == nil {
		return model.Artifact{}, fmt.Errorf("artifact уже существует: %s", path)
	} else if !os.IsNotExist(err) {
		return model.Artifact{}, fmt.Errorf("проверить artifact %s: %w", path, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return model.Artifact{}, fmt.Errorf("создать каталог artifact %s: %w", filepath.Dir(path), err)
	}

	tmp := path + ".tmp"
	file, err := os.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return model.Artifact{}, fmt.Errorf("создать временный artifact %s: %w", tmp, err)
	}

	hash := sha256.New()
	size, copyErr := io.Copy(io.MultiWriter(file, hash), body)
	closeErr := file.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return model.Artifact{}, fmt.Errorf("записать artifact %s: %w", tmp, copyErr)
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return model.Artifact{}, fmt.Errorf("закрыть artifact %s: %w", tmp, closeErr)
	}
	if err := ctx.Err(); err != nil {
		_ = os.Remove(tmp)
		return model.Artifact{}, err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return model.Artifact{}, fmt.Errorf("сохранить artifact %s: %w", path, err)
	}

	return model.Artifact{
		Path:         path,
		Name:         filepath.Base(path),
		ChecksumPath: path + ".sha256",
		Checksum:     hex.EncodeToString(hash.Sum(nil)),
		SizeBytes:    size,
	}, nil
}

func (FileStore) WriteChecksum(ctx context.Context, artifact model.Artifact) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := model.ValidateChecksum("checksum", artifact.Checksum); err != nil {
		return err
	}
	path := artifact.ChecksumPath
	if strings.TrimSpace(path) == "" {
		path = artifact.Path + ".sha256"
	}
	body := artifact.Checksum + "  " + filepath.Base(artifact.Path) + "\n"
	return writeTempFile(path, []byte(body), 0o644)
}

func (FileStore) WriteManifestJSON(ctx context.Context, path string, manifest model.CompleteManifest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := manifest.ValidateComplete(); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("собрать manifest json: %w", err)
	}
	raw = append(raw, '\n')
	return writeTempFile(path, raw, 0o644)
}

func (FileStore) WriteManifestText(ctx context.Context, path string, manifest model.CompleteManifest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := manifest.ValidateComplete(); err != nil {
		return err
	}
	var body strings.Builder
	createdAt, err := parseManifestCreatedAt(manifest.CreatedAt)
	if err != nil {
		return err
	}
	fmt.Fprintf(&body, "created_at=%s\n", model.FormatStamp(createdAt))
	fmt.Fprintf(&body, "contour=%s\n", manifest.Contour)
	fmt.Fprintf(&body, "compose_project=%s\n", manifest.ComposeProject)
	fmt.Fprintf(&body, "env_file=%s\n", manifest.EnvFile)
	fmt.Fprintf(&body, "espocrm_image=%s\n", manifest.EspoCRMImage)
	fmt.Fprintf(&body, "mariadb_tag=%s\n", manifest.MariaDBTag)
	fmt.Fprintf(&body, "retention_days=%d\n", manifest.RetentionDays)
	fmt.Fprintf(&body, "db_backup_created=1\n")
	fmt.Fprintf(&body, "files_backup_created=1\n")
	fmt.Fprintf(&body, "consistent_snapshot=%d\n", boolAsInt(manifest.ConsistentSnapshot))
	fmt.Fprintf(&body, "app_services_were_running=%d\n", boolAsInt(manifest.AppServicesWereRunning))
	fmt.Fprintf(&body, "db_backup_file=%s\n", manifest.Artifacts.DBBackup)
	fmt.Fprintf(&body, "db_backup_checksum_file=%s.sha256\n", manifest.Artifacts.DBBackup)
	fmt.Fprintf(&body, "db_backup_sha256=%s\n", manifest.Checksums.DBBackup)
	fmt.Fprintf(&body, "db_backup_size_bytes=%d\n", manifest.DBBackupSizeBytes)
	fmt.Fprintf(&body, "files_backup_file=%s\n", manifest.Artifacts.FilesBackup)
	fmt.Fprintf(&body, "files_backup_checksum_file=%s.sha256\n", manifest.Artifacts.FilesBackup)
	fmt.Fprintf(&body, "files_backup_sha256=%s\n", manifest.Checksums.FilesBackup)
	fmt.Fprintf(&body, "files_backup_size_bytes=%d\n", manifest.FilesBackupSizeBytes)
	return writeTempFile(path, []byte(body.String()), 0o644)
}

func (FileStore) ListBackupGroups(ctx context.Context, root string) ([]model.BackupGroup, error) {
	seen := map[model.BackupGroup]struct{}{}
	for _, dir := range []string{
		filepath.Join(root, "db"),
		filepath.Join(root, "files"),
		filepath.Join(root, "manifests"),
	} {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		entries, err := os.ReadDir(dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("прочитать каталог backup %s: %w", dir, err)
		}
		for _, entry := range entries {
			group, ok := model.ParseBackupGroupName(entry.Name())
			if ok {
				seen[group] = struct{}{}
			}
		}
	}

	groups := make([]model.BackupGroup, 0, len(seen))
	for group := range seen {
		groups = append(groups, group)
	}
	model.SortBackupGroups(groups)
	return groups, nil
}

func (FileStore) RemoveBackupSet(ctx context.Context, layout model.BackupLayout) error {
	for _, path := range layout.CompleteSetPaths() {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("удалить backup-set path %s: %w", path, err)
		}
	}
	return nil
}

func (FileStore) CleanupLayoutTemps(ctx context.Context, layout model.BackupLayout) error {
	var firstErr error
	for _, path := range layout.TempPaths() {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) && firstErr == nil {
			firstErr = fmt.Errorf("удалить временный backup path %s: %w", path, err)
		}
	}
	return firstErr
}

func writeTempFile(path string, body []byte, perm os.FileMode) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("путь файла обязателен")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("создать каталог файла %s: %w", filepath.Dir(path), err)
	}
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("файл уже существует: %s", path)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("проверить файл %s: %w", path, err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, body, perm); err != nil {
		return fmt.Errorf("записать временный файл %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("сохранить файл %s: %w", path, err)
	}
	return nil
}

func parseManifestCreatedAt(value string) (time.Time, error) {
	return time.Parse(time.RFC3339, value)
}

func boolAsInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
