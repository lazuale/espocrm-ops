package filesport

import "archive/tar"

type DirReadiness struct {
	Path        string
	ProbePath   string
	Exists      bool
	Creatable   bool
	Writable    bool
	FreeSpaceOK bool
}

type Stage interface {
	PreparedDir() string
	Cleanup() error
}

type Files interface {
	CreateTarGz(sourceDir, archivePath string) error
	SHA256File(path string) (string, error)
	InspectDirReadiness(path string, minFreeMB int, hasMinFree bool) (DirReadiness, error)
	EnsureNonEmptyFile(label, path string) (int64, error)
	EnsureWritableDir(path string) error
	EnsureFreeSpace(path string, neededBytes uint64) error
	NewSiblingStage(targetDir, prefix string) (Stage, error)
	UnpackTarGz(archivePath, destDir string, validateHeader func(*tar.Header) error) error
	PreparedTreeRoot(stageDir, targetBase string) (string, error)
	PreparedTreeRootExact(stageDir, targetBase string) (string, error)
	ReplaceTree(targetDir, preparedDir string) error
}
