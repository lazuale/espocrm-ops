package appadapter

import (
	"archive/tar"

	filesport "github.com/lazuale/espocrm-ops/internal/app/ports/filesport"
	platformfs "github.com/lazuale/espocrm-ops/internal/platform/fs"
)

type Files struct{}

func (Files) CreateTarGz(sourceDir, archivePath string) error {
	return platformfs.CreateTarGz(sourceDir, archivePath)
}

func (Files) SHA256File(path string) (string, error) {
	return platformfs.SHA256File(path)
}

func (Files) InspectDirReadiness(path string, minFreeMB int, hasMinFree bool) (filesport.DirReadiness, error) {
	readiness, err := platformfs.InspectDirReadiness(path, minFreeMB, hasMinFree)
	if err != nil {
		return filesport.DirReadiness{}, err
	}
	return filesport.DirReadiness{
		Path:        readiness.Path,
		ProbePath:   readiness.ProbePath,
		Exists:      readiness.Exists,
		Creatable:   readiness.Creatable,
		Writable:    readiness.Writable,
		FreeSpaceOK: readiness.FreeSpaceOK,
	}, nil
}

func (Files) EnsureNonEmptyFile(label, path string) (int64, error) {
	return platformfs.EnsureNonEmptyFile(label, path)
}

func (Files) EnsureWritableDir(path string) error {
	return platformfs.EnsureWritableDir(path)
}

func (Files) EnsureFreeSpace(path string, neededBytes uint64) error {
	return platformfs.EnsureFreeSpace(path, neededBytes)
}

func (Files) NewSiblingStage(targetDir, prefix string) (filesport.Stage, error) {
	stage, err := platformfs.NewSiblingStage(targetDir, prefix)
	if err != nil {
		return nil, err
	}
	return stageAdapter{stage: stage}, nil
}

func (Files) UnpackTarGz(archivePath, destDir string, validateHeader func(*tar.Header) error) error {
	return platformfs.UnpackTarGz(archivePath, destDir, validateHeader)
}

func (Files) PreparedTreeRoot(stageDir, targetBase string) (string, error) {
	return platformfs.PreparedTreeRoot(stageDir, targetBase)
}

func (Files) PreparedTreeRootExact(stageDir, targetBase string) (string, error) {
	return platformfs.PreparedTreeRootExact(stageDir, targetBase)
}

func (Files) ReplaceTree(targetDir, preparedDir string) error {
	return platformfs.ReplaceTree(targetDir, preparedDir)
}

type stageAdapter struct {
	stage platformfs.Stage
}

func (s stageAdapter) PreparedDir() string { return s.stage.PreparedDir }
func (s stageAdapter) Cleanup() error      { return s.stage.Cleanup() }
