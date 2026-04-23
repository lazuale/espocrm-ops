package store

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/lazuale/espocrm-ops/internal/model"
	platformfs "github.com/lazuale/espocrm-ops/internal/platform/fs"
)

func (s FileStore) LoadRestoreManifest(ctx context.Context, path string) (model.BackupVerifyManifest, error) {
	return s.LoadBackupVerifyManifest(ctx, path)
}

func (FileStore) RestoreFilesArtifact(ctx context.Context, filesBackupPath, targetDir string, requireExactRoot bool) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	filesBackupPath = strings.TrimSpace(filesBackupPath)
	if filesBackupPath == "" {
		return fmt.Errorf("files backup artifact обязателен")
	}
	targetDir = filepath.Clean(strings.TrimSpace(targetDir))
	if targetDir == "" || targetDir == "." || targetDir == string(filepath.Separator) {
		return fmt.Errorf("target storage dir недопустим: %s", targetDir)
	}

	stage, err := platformfs.NewSiblingStage(targetDir, "espops-restore-v2-files")
	if err != nil {
		return fmt.Errorf("подготовить staging для files restore: %w", err)
	}
	defer func() {
		if cleanupErr := stage.Cleanup(); cleanupErr != nil && err == nil {
			err = fmt.Errorf("очистить staging после files restore: %w", cleanupErr)
		}
	}()

	if err := platformfs.UnpackTarGz(filesBackupPath, stage.PreparedDir, model.ValidateFilesArchiveHeader); err != nil {
		return fmt.Errorf("распаковать files backup: %w", err)
	}

	var preparedDir string
	if requireExactRoot {
		preparedDir, err = platformfs.PreparedTreeRootExact(stage.PreparedDir, filepath.Base(targetDir))
	} else {
		preparedDir, err = platformfs.PreparedTreeRoot(stage.PreparedDir, filepath.Base(targetDir))
	}
	if err != nil {
		return fmt.Errorf("выбрать восстановленное дерево files: %w", err)
	}

	if err := platformfs.ReplaceTree(targetDir, preparedDir); err != nil {
		return fmt.Errorf("заменить storage tree: %w", err)
	}
	return nil
}
