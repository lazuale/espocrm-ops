package docker

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func CreateTarArchiveViaHelper(sourceDir, archivePath, mariaDBTag, espocrmImage string) error {
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

	image, err := selectCleanupHelperImage(mariaDBTag, espocrmImage)
	if err != nil {
		return err
	}

	if _, err := runCommand(commandOptions{
		Env: dockerCommandEnv(
			"ARCHIVE_SOURCE_BASENAME="+sourceBase,
			"ARCHIVE_OUTPUT_BASENAME="+archiveBase,
		),
	}, "docker",
		"run",
		"--pull=never",
		"--rm",
		"--entrypoint", "sh",
		"-v", sourceParent+":/archive-source:ro",
		"-v", archiveDir+":/archive-target",
		"-e", "ARCHIVE_SOURCE_BASENAME",
		"-e", "ARCHIVE_OUTPUT_BASENAME",
		image,
		"-euc", `tar -C /archive-source -czf "/archive-target/$ARCHIVE_OUTPUT_BASENAME" "$ARCHIVE_SOURCE_BASENAME"`,
	); err != nil {
		return fmt.Errorf("docker tar archive fallback: %w", err)
	}

	return nil
}

func selectCleanupHelperImage(mariaDBTag, espocrmImage string) (string, error) {
	candidates := []string{"alpine:3.20", "busybox:1.36"}
	if candidate := strings.TrimSpace(mariaDBTag); candidate != "" {
		candidates = append(candidates, "mariadb:"+candidate)
	}
	if candidate := strings.TrimSpace(espocrmImage); candidate != "" {
		candidates = append(candidates, candidate)
	}

	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if _, err := runCommand(commandOptions{Env: dockerCommandEnv()}, "docker", "image", "inspect", candidate); err == nil {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("no cleanup helper image is available locally")
}
