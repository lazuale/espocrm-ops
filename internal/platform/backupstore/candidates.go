package backupstore

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	domainbackup "github.com/lazuale/espocrm-ops/internal/domain/backup"
)

type ManifestCandidate struct {
	Prefix       string
	Stamp        string
	ManifestPath string
}

func ManifestCandidates(backupRoot string) ([]ManifestCandidate, error) {
	if strings.TrimSpace(backupRoot) == "" {
		return nil, fmt.Errorf("backup root is required")
	}

	manifestDir := filepath.Join(backupRoot, "manifests")
	entries, err := os.ReadDir(manifestDir)
	if err != nil {
		return nil, fmt.Errorf("read manifests dir: %w", err)
	}

	candidates := []ManifestCandidate{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		group, kind, err := domainbackup.ParseManifestName(entry.Name())
		if err != nil || kind != domainbackup.ManifestJSON {
			continue
		}

		set := domainbackup.BuildBackupSet(backupRoot, group.Prefix, group.Stamp)
		candidates = append(candidates, ManifestCandidate{
			Prefix:       group.Prefix,
			Stamp:        group.Stamp,
			ManifestPath: set.ManifestJSON.Path,
		})
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Stamp == candidates[j].Stamp {
			return candidates[i].Prefix > candidates[j].Prefix
		}
		return candidates[i].Stamp > candidates[j].Stamp
	})

	return candidates, nil
}
