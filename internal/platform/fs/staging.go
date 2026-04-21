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
		return Stage{}, ensureDirError{Path: parent, Err: err}
	}

	workDir, err := os.MkdirTemp(parent, "."+prefix+".")
	if err != nil {
		return Stage{}, stageCreateRootError{Path: parent, Err: err}
	}

	preparedDir := filepath.Join(workDir, "stage")
	if err := os.MkdirAll(preparedDir, 0o755); err != nil {
		_ = os.RemoveAll(workDir)
		return Stage{}, stagePrepareDirError{Path: preparedDir, Err: err}
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
	return preparedTreeRoot(stageDir, targetBase, false)
}

func PreparedTreeRootExact(stageDir, targetBase string) (string, error) {
	return preparedTreeRoot(stageDir, targetBase, true)
}

func preparedTreeRoot(stageDir, targetBase string, requireExactRoot bool) (string, error) {
	entries, err := os.ReadDir(stageDir)
	if err != nil {
		return "", stageReadError{Path: stageDir, Err: err}
	}
	if len(entries) == 0 {
		return "", stageEmptyError{Path: stageDir}
	}

	if len(entries) == 1 && entries[0].IsDir() && entries[0].Name() == targetBase {
		return filepath.Join(stageDir, entries[0].Name()), nil
	}

	if requireExactRoot {
		return "", stageRootMismatchError{Path: stageDir, TargetBase: targetBase}
	}

	for _, entry := range entries {
		if entry.Name() == targetBase {
			return "", stageMixedRootError{Path: stageDir, TargetBase: targetBase}
		}
	}

	return stageDir, nil
}
