package backup

import (
	"fmt"

	"github.com/lazuale/espocrm-ops/internal/platform/backupstore"
)

type ManifestCandidate struct {
	Prefix       string
	Stamp        string
	ManifestPath string
}

func LatestCompleteManifest(backupRoot string) (string, error) {
	candidates, err := ManifestCandidates(backupRoot)
	if err != nil {
		return "", err
	}

	for _, candidate := range candidates {
		if _, err := VerifyDetailed(VerifyRequest{ManifestPath: candidate.ManifestPath}); err == nil {
			return candidate.ManifestPath, nil
		}
	}

	return "", ValidationError{Err: fmt.Errorf("no complete backup set found in %s", backupRoot)}
}

func ManifestCandidates(backupRoot string) ([]ManifestCandidate, error) {
	candidates, err := backupstore.ManifestCandidates(backupRoot)
	if err != nil {
		return nil, err
	}

	out := make([]ManifestCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		out = append(out, ManifestCandidate{
			Prefix:       candidate.Prefix,
			Stamp:        candidate.Stamp,
			ManifestPath: candidate.ManifestPath,
		})
	}

	return out, nil
}
