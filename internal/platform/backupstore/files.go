package backupstore

import (
	"os"
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
