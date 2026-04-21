package backupstore

import (
	"os"
	"time"
)

type fileInfo struct {
	Path    string
	Exists  bool
	IsDir   bool
	Size    int64
	ModTime time.Time
}

func inspectFile(path string) (fileInfo, error) {
	stat, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fileInfo{Path: path}, nil
		}
		return fileInfo{}, err
	}

	return fileInfo{
		Path:    path,
		Exists:  true,
		IsDir:   stat.IsDir(),
		Size:    stat.Size(),
		ModTime: stat.ModTime(),
	}, nil
}
