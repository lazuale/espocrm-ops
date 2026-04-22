package docker

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func CreateTarArchiveViaHelper(sourceDir, archivePath, helperImage string) error {
	if strings.TrimSpace(sourceDir) == "" {
		return fmt.Errorf("source dir is required")
	}
	if strings.TrimSpace(archivePath) == "" {
		return fmt.Errorf("archive path is required")
	}

	sourceParent := filepath.Dir(sourceDir)
	sourceBase := filepath.Base(sourceDir)
	archiveDir := filepath.Dir(archivePath)
	archiveBase := filepath.Base(archivePath)

	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		return fmt.Errorf("ensure archive dir: %w", err)
	}

	helperImage, err := ensureHelperImageAvailable(helperImage)
	if err != nil {
		return err
	}

	if _, err := runDockerCommand(
		"run",
		"--pull=never",
		"--rm",
		"--entrypoint", "tar",
		"-v", sourceParent+":/archive-source:ro",
		"-v", archiveDir+":/archive-target",
		helperImage,
		"-C", "/archive-source",
		"-czf", filepath.Join("/archive-target", archiveBase),
		sourceBase,
	); err != nil {
		return fmt.Errorf("docker tar archive fallback: %w", err)
	}

	return nil
}

func ensureHelperImageAvailable(helperImage string) (string, error) {
	image := strings.TrimSpace(helperImage)
	if image == "" {
		return "", fmt.Errorf("ESPO_HELPER_IMAGE is required")
	}

	if _, err := runDockerCommand("image", "inspect", image); err != nil {
		if isDockerImageMissing(err) {
			return "", fmt.Errorf("helper image %s is not available locally", image)
		}
		return "", fmt.Errorf("inspect helper image %s: %w", image, err)
	}

	return image, nil
}
