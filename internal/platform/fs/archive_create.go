package fs

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func CreateTarGz(sourceDir, archivePath string) error {
	if strings.TrimSpace(sourceDir) == "" {
		return fmt.Errorf("source dir is required")
	}
	if strings.TrimSpace(archivePath) == "" {
		return fmt.Errorf("archive path is required")
	}

	if err := os.MkdirAll(filepath.Dir(archivePath), 0o755); err != nil {
		return fmt.Errorf("ensure archive dir: %w", err)
	}

	var stderr bytes.Buffer
	cmd := exec.Command("tar", "-C", filepath.Dir(sourceDir), "-czf", archivePath, filepath.Base(sourceDir))
	cmd.Stdout = io.Discard
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("create tar archive: %w%s", err, tarCommandErrorSuffix(stderr.String()))
	}

	return nil
}

func tarCommandErrorSuffix(stderr string) string {
	if line := tarLastNonBlankLine(stderr); line != "" {
		return ": " + line
	}

	return ""
}

func tarLastNonBlankLine(text string) string {
	text = strings.ReplaceAll(text, "\r", "")
	lines := strings.Split(text, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			return line
		}
	}

	return ""
}
