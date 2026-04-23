package resultbridge

import (
	"fmt"
	"io"

	"github.com/lazuale/espocrm-ops/internal/contract/result"
	"github.com/lazuale/espocrm-ops/internal/model"
)

// Временный cutover shim: CLI пока использует общий result transport,
// а v2 core возвращает собственный model-result. После удаления старого
// backup verify path этот bridge можно сузить вместе с остальными resultbridge.
func BackupVerifyResult(info model.BackupVerifyResult) result.Result {
	message := "backup verify completed"
	if !info.OK {
		message = "backup verify failed"
	}
	exitCode := info.ProcessExitCode
	items := make([]result.ItemPayload, 0, len(info.Items))
	for _, step := range info.Items {
		items = append(items, result.BackupVerifyItem{
			SectionItem: result.SectionItem{
				Code:    step.Code,
				Status:  step.Status,
				Summary: backupVerifyStepSummary(step.Code, step.Status),
			},
		})
	}

	var errorInfo *result.ErrorInfo
	if info.Error != nil {
		errorInfo = &result.ErrorInfo{
			Code:     info.Error.Code,
			Kind:     string(info.Error.Kind),
			ExitCode: info.Error.ExitCode,
			Message:  info.Error.Message,
		}
	}

	return result.Result{
		Command:         "backup verify",
		OK:              info.OK,
		ProcessExitCode: &exitCode,
		Message:         message,
		Error:           errorInfo,
		Artifacts: result.BackupVerifyArtifacts{
			BackupRoot:    info.Artifacts.BackupRoot,
			Manifest:      info.Artifacts.Manifest,
			DBBackup:      info.Artifacts.DBBackup,
			DBChecksum:    info.Artifacts.DBChecksum,
			FilesBackup:   info.Artifacts.FilesBackup,
			FilesChecksum: info.Artifacts.FilesChecksum,
		},
		Details: result.BackupVerifyDetails{
			Ready:      info.Details.Ready,
			SourceKind: info.Details.SourceKind,
			Scope:      info.Details.Scope,
			CreatedAt:  info.Details.CreatedAt,
			Steps:      info.Details.Steps,
			Completed:  info.Details.Completed,
			Skipped:    info.Details.Skipped,
			Blocked:    info.Details.Blocked,
			Failed:     info.Details.Failed,
		},
		Items: items,
	}
}

func RenderBackupVerifyText(w io.Writer, res result.Result) error {
	if !res.OK {
		return result.Render(w, res, false)
	}

	artifacts, ok := res.Artifacts.(result.BackupVerifyArtifacts)
	if !ok {
		return result.Render(w, res, false)
	}

	if _, err := fmt.Fprintln(w, "Проверка backup-set"); err != nil {
		return err
	}
	if artifacts.BackupRoot != "" {
		if _, err := fmt.Fprintf(w, "Backup root: %s\n", artifacts.BackupRoot); err != nil {
			return err
		}
	}
	if artifacts.DBBackup != "" {
		if _, err := fmt.Fprintf(w, "Backup базы данных: %s\n", artifacts.DBBackup); err != nil {
			return err
		}
	}
	if artifacts.FilesBackup != "" {
		if _, err := fmt.Fprintf(w, "Backup файлов: %s\n", artifacts.FilesBackup); err != nil {
			return err
		}
	}
	if artifacts.Manifest != "" {
		if _, err := fmt.Fprintf(w, "JSON manifest: %s\n", artifacts.Manifest); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w, "[ok] Backup-set: manifest, checksums и читаемость archives подтверждены"); err != nil {
		return err
	}
	_, err := fmt.Fprintln(w, "Проверка backup завершена успешно")
	return err
}

func backupVerifyStepSummary(code, status string) string {
	if status == model.StatusBlocked {
		switch code {
		case model.StepVerifyManifest:
			return "Проверка manifest заблокирована"
		case model.StepVerifyDBArtifact:
			return "Проверка database artifact заблокирована"
		case model.StepVerifyFilesArtifact:
			return "Проверка files artifact заблокирована"
		}
	}

	switch code {
	case model.StepVerifySource:
		return "Источник backup verify выбран"
	case model.StepVerifyManifest:
		return "Manifest полного backup-set проверен"
	case model.StepVerifyDBArtifact:
		return "Database artifact проверен"
	case model.StepVerifyFilesArtifact:
		return "Files artifact проверен"
	default:
		return code
	}
}
