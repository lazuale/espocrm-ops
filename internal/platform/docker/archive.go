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

	image, err := selectLocalHelperImage(mariaDBTag, espocrmImage)
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
		image,
		"-C", "/archive-source",
		"-czf", filepath.Join("/archive-target", archiveBase),
		sourceBase,
	); err != nil {
		return fmt.Errorf("docker tar archive fallback: %w", err)
	}

	return nil
}

func selectLocalHelperImage(mariaDBTag, espocrmImage string) (string, error) {
	candidates := helperImageCandidates(mariaDBTag, espocrmImage)
	if len(candidates) == 0 {
		return "", fmt.Errorf("no helper image candidates configured")
	}

	for _, candidate := range candidates {
		if _, err := runDockerCommand("image", "inspect", candidate); err == nil {
			return candidate, nil
		} else if !isDockerImageMissing(err) {
			return "", fmt.Errorf("inspect helper image %s: %w", candidate, err)
		}
	}

	return "", fmt.Errorf("no local helper image is available (checked: %s)", strings.Join(candidates, ", "))
}

func helperImageCandidates(mariaDBTag, espocrmImage string) []string {
	candidates := make([]string, 0, 4)
	appendUniqueHelperImageCandidate(&candidates, strings.TrimSpace(espocrmImage))
	if tag := strings.TrimSpace(mariaDBTag); tag != "" {
		appendUniqueHelperImageCandidate(&candidates, "mariadb:"+tag)
	}
	appendUniqueHelperImageCandidate(&candidates, "alpine:3.20")
	appendUniqueHelperImageCandidate(&candidates, "busybox:1.36")
	return candidates
}

func appendUniqueHelperImageCandidate(candidates *[]string, candidate string) {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return
	}

	for _, existing := range *candidates {
		if existing == candidate {
			return
		}
	}

	*candidates = append(*candidates, candidate)
}
