package backup

import (
	domainbackup "github.com/lazuale/espocrm-ops/internal/domain/backup"
	"github.com/lazuale/espocrm-ops/internal/platform/backupstore"
)

func LoadManifest(path string) (domainbackup.Manifest, error) {
	manifest, err := backupstore.LoadManifest(path)
	if err != nil {
		return domainbackup.Manifest{}, classifyStoreError(err)
	}

	return manifest, nil
}
