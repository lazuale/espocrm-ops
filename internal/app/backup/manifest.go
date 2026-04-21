package backup

import (
	domainbackup "github.com/lazuale/espocrm-ops/internal/domain/backup"
)

func (s Service) LoadManifest(path string) (domainbackup.Manifest, error) {
	manifest, err := s.store.LoadManifest(path)
	if err != nil {
		return domainbackup.Manifest{}, err
	}

	return manifest, nil
}
