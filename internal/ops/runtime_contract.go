package ops

import (
	"fmt"
	"slices"
	"strings"

	config "github.com/lazuale/espocrm-ops/internal/config"
	manifestpkg "github.com/lazuale/espocrm-ops/internal/manifest"
)

func backupManifestRuntime(cfg config.BackupConfig) manifestpkg.Runtime {
	return manifestpkg.Runtime{
		EspoCRMImage:     cfg.EspoCRMImage,
		MariaDBImage:     cfg.MariaDBImage,
		DBName:           cfg.DBName,
		DBService:        cfg.DBService,
		AppServices:      append([]string(nil), cfg.AppServices...),
		BackupNamePrefix: cfg.BackupNamePrefix,
		StorageContract:  manifestpkg.StorageContractEspoCRMFullStorageV1,
	}
}

func requireRestoreRuntimeContract(cfg config.BackupConfig, result VerifyResult) error {
	loadedManifest := manifestpkg.Manifest{
		Version: result.ManifestVersion,
		Runtime: result.Runtime,
	}
	if err := manifestpkg.RequireRestoreRuntimeContract(loadedManifest); err != nil {
		return manifestError("manifest runtime contract is invalid", err)
	}

	expected := backupManifestRuntime(cfg)
	for _, check := range []struct {
		name     string
		actual   string
		expected string
		envKey   string
	}{
		{
			name:     "runtime.espo_crm_image",
			actual:   result.Runtime.EspoCRMImage,
			expected: expected.EspoCRMImage,
			envKey:   "ESPOCRM_IMAGE",
		},
		{
			name:     "runtime.mariadb_image",
			actual:   result.Runtime.MariaDBImage,
			expected: expected.MariaDBImage,
			envKey:   "MARIADB_IMAGE",
		},
		{
			name:     "runtime.db_service",
			actual:   result.Runtime.DBService,
			expected: expected.DBService,
			envKey:   "DB_SERVICE",
		},
		{
			name:     "runtime.storage_contract",
			actual:   result.Runtime.StorageContract,
			expected: expected.StorageContract,
			envKey:   "storage contract",
		},
	} {
		if check.actual == check.expected {
			continue
		}
		return manifestError(
			"manifest runtime contract is invalid",
			fmt.Errorf(
				"%s %q does not match target %s %q",
				check.name,
				check.actual,
				check.envKey,
				check.expected,
			),
		)
	}

	if strings.TrimSpace(result.Scope) == strings.TrimSpace(cfg.Scope) && result.Runtime.DBName != expected.DBName {
		return manifestError(
			"manifest runtime contract is invalid",
			fmt.Errorf(
				`runtime.db_name %q does not match target DB_NAME %q`,
				result.Runtime.DBName,
				expected.DBName,
			),
		)
	}

	if !slices.Equal(result.Runtime.AppServices, expected.AppServices) {
		return manifestError(
			"manifest runtime contract is invalid",
			fmt.Errorf(
				`runtime.app_services %q does not match target APP_SERVICES %q`,
				strings.Join(result.Runtime.AppServices, ","),
				strings.Join(expected.AppServices, ","),
			),
		)
	}

	return nil
}
