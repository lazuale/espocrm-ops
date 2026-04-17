package fs

import (
	"os"
	"path/filepath"
)

type Stage struct {
	WorkDir     string
	PreparedDir string
}

func NewSiblingStage(targetDir, prefix string) (Stage, error) {
	if prefix == "" {
		prefix = "espops-stage"
	}

	parent := filepath.Dir(targetDir)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return Stage{}, EnsureDirError{Path: parent, Err: err}
	}

	workDir, err := os.MkdirTemp(parent, "."+prefix+".")
	if err != nil {
		return Stage{}, StageCreateRootError{Path: parent, Err: err}
	}

	preparedDir := filepath.Join(workDir, "stage")
	if err := os.MkdirAll(preparedDir, 0o755); err != nil {
		_ = os.RemoveAll(workDir)
		return Stage{}, StagePrepareDirError{Path: preparedDir, Err: err}
	}

	return Stage{
		WorkDir:     workDir,
		PreparedDir: preparedDir,
	}, nil
}

func (s Stage) Cleanup() error {
	if s.WorkDir == "" {
		return nil
	}

	return os.RemoveAll(s.WorkDir)
}

func PreparedTreeRoot(stageDir, targetBase string) (string, error) {
	entries, err := os.ReadDir(stageDir)
	if err != nil {
		return "", StageReadError{Path: stageDir, Err: err}
	}
	if len(entries) == 0 {
		return "", StageEmptyError{Path: stageDir}
	}

	if len(entries) == 1 && entries[0].IsDir() && entries[0].Name() == targetBase {
		return filepath.Join(stageDir, entries[0].Name()), nil
	}

	for _, entry := range entries {
		if entry.Name() == targetBase {
			return "", StageMixedRootError{Path: stageDir, TargetBase: targetBase}
		}
	}

	return stageDir, nil
}

func PreparedTreeRootExact(stageDir, targetBase string) (string, error) {
	entries, err := os.ReadDir(stageDir)
	if err != nil {
		return "", StageReadError{Path: stageDir, Err: err}
	}
	if len(entries) == 0 {
		return "", StageEmptyError{Path: stageDir}
	}
	if len(entries) != 1 || !entries[0].IsDir() || entries[0].Name() != targetBase {
		return "", StageRootMismatchError{Path: stageDir, TargetBase: targetBase}
	}

	return filepath.Join(stageDir, entries[0].Name()), nil
}
