package backupstore

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	domainbackup "github.com/lazuale/espocrm-ops/internal/domain/backup"
)

type GroupMode int

const (
	GroupModeAny GroupMode = iota
	GroupModeDB
	GroupModeFiles
	GroupModeManifests
)

func Groups(backupRoot string, mode GroupMode) ([]domainbackup.BackupGroup, error) {
	seen := map[string]domainbackup.BackupGroup{}

	collect := func(dir string, parse func(string) (domainbackup.BackupGroup, error)) error {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return fmt.Errorf("read catalog dir %s: %w", dir, err)
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			group, err := parse(entry.Name())
			if err != nil {
				continue
			}
			seen[groupKey(group)] = group
		}
		return nil
	}

	if mode == GroupModeAny || mode == GroupModeDB {
		if err := collect(filepath.Join(backupRoot, "db"), domainbackup.ParseDBBackupName); err != nil {
			return nil, err
		}
	}
	if mode == GroupModeAny || mode == GroupModeFiles {
		if err := collect(filepath.Join(backupRoot, "files"), domainbackup.ParseFilesBackupName); err != nil {
			return nil, err
		}
	}
	if mode == GroupModeAny || mode == GroupModeManifests {
		if err := collect(filepath.Join(backupRoot, "manifests"), func(name string) (domainbackup.BackupGroup, error) {
			group, _, err := domainbackup.ParseManifestName(name)
			return group, err
		}); err != nil {
			return nil, err
		}
	}

	groups := make([]domainbackup.BackupGroup, 0, len(seen))
	for _, group := range seen {
		groups = append(groups, group)
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].Stamp == groups[j].Stamp {
			return groups[i].Prefix > groups[j].Prefix
		}
		return groups[i].Stamp > groups[j].Stamp
	})

	return groups, nil
}

func groupKey(group domainbackup.BackupGroup) string {
	return group.Prefix + "|" + group.Stamp
}
