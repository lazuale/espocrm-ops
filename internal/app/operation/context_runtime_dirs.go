package operation

import (
	"fmt"

	"github.com/lazuale/espocrm-ops/internal/domain/env"
)

func (s Service) verifyRuntimePaths(projectDir string, values env.OperationEnv) error {
	paths := []string{
		s.env.ResolveProjectPath(projectDir, values.DBStorageDir()),
		s.env.ResolveProjectPath(projectDir, values.ESPOStorageDir()),
		s.env.ResolveProjectPath(projectDir, values.BackupRoot()),
	}

	for _, path := range paths {
		readiness, err := s.files.InspectDirReadiness(path, 0, false)
		if err != nil {
			return fmt.Errorf("inspect runtime path %s: %w", path, err)
		}
		if readiness.Writable {
			continue
		}

		target := readiness.Path
		if readiness.ProbePath != "" {
			target = readiness.ProbePath
		}
		return fmt.Errorf("runtime path %s is not writable via %s", readiness.Path, target)
	}

	return nil
}
