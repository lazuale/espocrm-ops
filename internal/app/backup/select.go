package backup

import (
	"fmt"

	domainfailure "github.com/lazuale/espocrm-ops/internal/domain/failure"
)

type ManifestCandidate struct {
	Prefix       string
	Stamp        string
	ManifestPath string
}

func (s Service) LatestCompleteManifest(backupRoot string) (string, error) {
	candidates, err := s.ManifestCandidates(backupRoot)
	if err != nil {
		return "", err
	}

	for _, candidate := range candidates {
		if _, err := s.VerifyDetailed(VerifyRequest{ManifestPath: candidate.ManifestPath}); err == nil {
			return candidate.ManifestPath, nil
		}
	}

	return "", domainfailure.Failure{
		Kind: domainfailure.KindValidation,
		Code: "backup_verification_failed",
		Err:  fmt.Errorf("no complete backup set found in %s", backupRoot),
	}
}

func (s Service) ManifestCandidates(backupRoot string) ([]ManifestCandidate, error) {
	candidates, err := s.store.ManifestCandidates(backupRoot)
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
