package backupstore

import (
	"fmt"
	"os"
	"strings"
	"time"
)

type FileInfo struct {
	Path    string
	Exists  bool
	IsDir   bool
	Size    int64
	ModTime time.Time
}

func InspectFile(path string) (FileInfo, error) {
	stat, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return FileInfo{Path: path}, nil
		}
		return FileInfo{}, err
	}

	return FileInfo{
		Path:    path,
		Exists:  true,
		IsDir:   stat.IsDir(),
		Size:    stat.Size(),
		ModTime: stat.ModTime(),
	}, nil
}

func ValidateTXTManifest(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	text := string(raw)
	for _, key := range []string{"created_at=", "contour=", "compose_project="} {
		if !strings.Contains(text, key) {
			return fmt.Errorf("missing %s", strings.TrimSuffix(key, "="))
		}
	}

	return nil
}
