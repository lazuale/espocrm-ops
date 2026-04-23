package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/lazuale/espocrm-ops/internal/model"
)

type BackupVerifyDependencies struct {
	Store model.BackupVerifyStore
}

type BackupVerifyService struct {
	store model.BackupVerifyStore
}

func NewBackupVerifyService(deps BackupVerifyDependencies) BackupVerifyService {
	return BackupVerifyService{store: deps.Store}
}

func (s BackupVerifyService) VerifyBackup(ctx context.Context, req model.BackupVerifyRequest) (result model.BackupVerifyResult, err error) {
	req = normalizeBackupVerifyRequest(req)
	result = model.NewBackupVerifyResult(req)

	if validateErr := validateBackupVerifyRequest(req); validateErr != nil {
		failure := model.NewBackupVerifyFailure(model.KindUsage, model.BackupVerifyFailedCode, validateErr.Error(), validateErr)
		result.Fail(failure)
		return result, failure
	}
	if s.store == nil {
		failure := model.NewBackupVerifyFailure(model.KindValidation, model.BackupVerifyFailedCode, "store не настроен", nil)
		result.Fail(failure)
		return result, failure
	}

	switch {
	case req.ManifestPath != "":
		report, verifyErr := s.verifyManifestSet(ctx, req.ManifestPath, &result)
		if verifyErr != nil {
			failure := backupVerifyFailureFromError(verifyErr)
			result.Fail(failure)
			return result, failure
		}
		report.SourceKind = model.VerifySourceManifest
		result.ApplyReport(report)
	case req.BackupRoot != "":
		report, verifyErr := s.verifyLatestCompleteSet(ctx, req.BackupRoot, &result)
		if verifyErr != nil {
			failure := backupVerifyFailureFromError(verifyErr)
			result.Fail(failure)
			return result, failure
		}
		report.SourceKind = model.VerifySourceBackupRoot
		result.ApplyReport(report)
	case req.DBBackupPath != "":
		artifact, verifyErr := s.store.VerifyDBArtifact(ctx, req.DBBackupPath, "")
		if verifyErr != nil {
			result.AddStep(model.StepVerifyDBArtifact, model.StatusFailed)
			failure := backupVerifyFailureFromError(verifyErr)
			result.Fail(failure)
			return result, failure
		}
		result.AddStep(model.StepVerifyDBArtifact, model.StatusCompleted)
		result.ApplyReport(model.BackupVerifyReport{
			SourceKind: model.VerifySourceDirectDB,
			DBBackup:   artifact,
		})
	case req.FilesPath != "":
		artifact, verifyErr := s.store.VerifyFilesArtifact(ctx, req.FilesPath, "")
		if verifyErr != nil {
			result.AddStep(model.StepVerifyFilesArtifact, model.StatusFailed)
			failure := backupVerifyFailureFromError(verifyErr)
			result.Fail(failure)
			return result, failure
		}
		result.AddStep(model.StepVerifyFilesArtifact, model.StatusCompleted)
		result.ApplyReport(model.BackupVerifyReport{
			SourceKind:  model.VerifySourceDirectFiles,
			FilesBackup: artifact,
		})
	}

	result.Succeed()
	return result, nil
}

func (s BackupVerifyService) verifyLatestCompleteSet(ctx context.Context, backupRoot string, result *model.BackupVerifyResult) (model.BackupVerifyReport, error) {
	candidates, err := s.store.ListBackupVerifyManifestCandidates(ctx, backupRoot)
	if err != nil {
		result.AddStep(model.StepVerifySource, model.StatusFailed)
		blockBackupVerifyAfter(result, model.StepVerifySource)
		return model.BackupVerifyReport{}, err
	}

	for _, candidate := range candidates {
		scratch := model.NewBackupVerifyResult(model.BackupVerifyRequest{ManifestPath: candidate})
		report, err := s.verifyManifestSet(ctx, candidate, &scratch)
		if err != nil {
			continue
		}
		result.AddStep(model.StepVerifySource, model.StatusCompleted)
		for _, step := range scratch.Items {
			result.AddStep(step.Code, step.Status)
		}
		report.BackupRoot = backupRoot
		return report, nil
	}

	result.AddStep(model.StepVerifySource, model.StatusFailed)
	blockBackupVerifyAfter(result, model.StepVerifySource)
	return model.BackupVerifyReport{}, model.BackupVerifySelectionError{Err: fmt.Errorf("в backup root нет проверяемого complete backup-set")}
}

func (s BackupVerifyService) verifyManifestSet(ctx context.Context, manifestPath string, result *model.BackupVerifyResult) (model.BackupVerifyReport, error) {
	manifest, err := s.store.LoadBackupVerifyManifest(ctx, manifestPath)
	if err != nil {
		result.AddStep(model.StepVerifyManifest, model.StatusFailed)
		blockBackupVerifyAfter(result, model.StepVerifyManifest)
		return model.BackupVerifyReport{}, err
	}
	result.AddStep(model.StepVerifyManifest, model.StatusCompleted)

	dbPath := manifest.DBArtifactPath(manifestPath)
	dbArtifact, err := s.store.VerifyDBArtifact(ctx, dbPath, manifest.Checksums.DBBackup)
	if err != nil {
		result.AddStep(model.StepVerifyDBArtifact, model.StatusFailed)
		blockBackupVerifyAfter(result, model.StepVerifyDBArtifact)
		return model.BackupVerifyReport{}, err
	}
	result.AddStep(model.StepVerifyDBArtifact, model.StatusCompleted)

	filesPath := manifest.FilesArtifactPath(manifestPath)
	filesArtifact, err := s.store.VerifyFilesArtifact(ctx, filesPath, manifest.Checksums.FilesBackup)
	if err != nil {
		result.AddStep(model.StepVerifyFilesArtifact, model.StatusFailed)
		return model.BackupVerifyReport{}, err
	}
	result.AddStep(model.StepVerifyFilesArtifact, model.StatusCompleted)

	return model.BackupVerifyReport{
		ManifestPath: manifestPath,
		BackupRoot:   filepath.Dir(filepath.Dir(manifestPath)),
		Scope:        manifest.Scope,
		CreatedAt:    manifest.CreatedAt,
		DBBackup:     dbArtifact,
		FilesBackup:  filesArtifact,
	}, nil
}

func normalizeBackupVerifyRequest(req model.BackupVerifyRequest) model.BackupVerifyRequest {
	req.ManifestPath = strings.TrimSpace(req.ManifestPath)
	req.BackupRoot = strings.TrimSpace(req.BackupRoot)
	req.DBBackupPath = strings.TrimSpace(req.DBBackupPath)
	req.FilesPath = strings.TrimSpace(req.FilesPath)
	return req
}

func validateBackupVerifyRequest(req model.BackupVerifyRequest) error {
	var selected []string
	for _, value := range []string{req.ManifestPath, req.BackupRoot, req.DBBackupPath, req.FilesPath} {
		if strings.TrimSpace(value) != "" {
			selected = append(selected, value)
		}
	}
	switch len(selected) {
	case 0:
		return fmt.Errorf("нужно указать manifest, backup root или direct artifact")
	case 1:
		return nil
	default:
		return fmt.Errorf("можно указать только один источник verify")
	}
}

func backupVerifyFailureFromError(err error) model.BackupFailure {
	var manifestErr model.BackupVerifyManifestError
	if errors.As(err, &manifestErr) {
		return model.NewBackupVerifyFailure(model.KindManifest, model.ManifestInvalidCode, "manifest verify завершился ошибкой", err)
	}

	var selectionErr model.BackupVerifySelectionError
	if errors.As(err, &selectionErr) {
		return model.NewBackupVerifyFailure(model.KindValidation, model.BackupVerifyFailedCode, "backup root verify не нашёл complete backup-set", err)
	}

	var artifactErr model.BackupVerifyArtifactError
	if errors.As(err, &artifactErr) {
		return model.NewBackupVerifyFailure(model.KindValidation, model.BackupVerifyFailedCode, "backup artifact verify завершился ошибкой", err)
	}

	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return model.NewBackupVerifyFailure(model.KindExternal, model.BackupVerifyFailedCode, "backup verify прерван", err)
	}

	return model.NewBackupVerifyFailure(model.KindIO, model.BackupVerifyFailedCode, "backup verify завершился ошибкой", err)
}

func blockBackupVerifyAfter(result *model.BackupVerifyResult, failedStep string) {
	steps := []string{
		model.StepVerifySource,
		model.StepVerifyManifest,
		model.StepVerifyDBArtifact,
		model.StepVerifyFilesArtifact,
	}
	seen := false
	for _, step := range steps {
		if step == failedStep {
			seen = true
			continue
		}
		if !seen {
			continue
		}
		if backupVerifyHasStep(*result, step) {
			continue
		}
		result.AddStep(step, model.StatusBlocked)
	}
}

func backupVerifyHasStep(result model.BackupVerifyResult, code string) bool {
	for _, step := range result.Items {
		if step.Code == code {
			return true
		}
	}
	return false
}
