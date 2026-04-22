package env

import (
	"fmt"
	"strconv"
	"strings"
)

type Values interface {
	Value(key string) string
	ComposeProject() string
}

type ValueMismatch struct {
	Name       string
	LeftValue  string
	RightValue string
}

type RuntimeContract struct {
	HelperImage string
	UID         int
	GID         int
}

var migrationCompatibilityKeys = []string{
	"ESPOCRM_IMAGE",
	"MARIADB_TAG",
	"ESPO_DEFAULT_LANGUAGE",
	"ESPO_TIME_ZONE",
}

func MigrationCompatibilityMismatches(left, right Values) []ValueMismatch {
	mismatches := make([]ValueMismatch, 0, len(migrationCompatibilityKeys))

	for _, key := range migrationCompatibilityKeys {
		leftValue := left.Value(key)
		rightValue := right.Value(key)
		if leftValue == rightValue {
			continue
		}
		mismatches = append(mismatches, ValueMismatch{
			Name:       key,
			LeftValue:  leftValue,
			RightValue: rightValue,
		})
	}

	return mismatches
}

func BackupNamePrefix(values Values) string {
	if value := strings.TrimSpace(values.Value("BACKUP_NAME_PREFIX")); value != "" {
		return value
	}

	return strings.TrimSpace(values.ComposeProject())
}

func BackupRetentionDays(values Values) (int, error) {
	value := strings.TrimSpace(values.Value("BACKUP_RETENTION_DAYS"))
	if value == "" {
		return 7, nil
	}

	var days int
	if _, err := fmt.Sscanf(value, "%d", &days); err != nil {
		return 0, fmt.Errorf("BACKUP_RETENTION_DAYS must be an integer")
	}
	if days < 0 {
		return 0, fmt.Errorf("BACKUP_RETENTION_DAYS must be non-negative")
	}

	return days, nil
}

func ResolveRuntimeContract(values Values) (RuntimeContract, error) {
	helperImage := strings.TrimSpace(values.Value("ESPO_HELPER_IMAGE"))
	if helperImage == "" {
		return RuntimeContract{}, fmt.Errorf("ESPO_HELPER_IMAGE is required")
	}

	uid, err := parseNonNegativeInt("ESPO_RUNTIME_UID", values.Value("ESPO_RUNTIME_UID"))
	if err != nil {
		return RuntimeContract{}, err
	}

	gid, err := parseNonNegativeInt("ESPO_RUNTIME_GID", values.Value("ESPO_RUNTIME_GID"))
	if err != nil {
		return RuntimeContract{}, err
	}

	return RuntimeContract{
		HelperImage: helperImage,
		UID:         uid,
		GID:         gid,
	}, nil
}

func parseNonNegativeInt(name, raw string) (int, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, fmt.Errorf("%s is required", name)
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer", name)
	}
	if parsed < 0 {
		return 0, fmt.Errorf("%s must be non-negative", name)
	}

	return parsed, nil
}
