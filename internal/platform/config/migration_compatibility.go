package config

var migrationCompatibilityKeys = []string{
	"ESPOCRM_IMAGE",
	"MARIADB_TAG",
	"ESPO_DEFAULT_LANGUAGE",
	"ESPO_TIME_ZONE",
}

type EnvValueMismatch struct {
	Name       string
	LeftValue  string
	RightValue string
}

func MigrationCompatibilityMismatches(left, right OperationEnv) []EnvValueMismatch {
	mismatches := make([]EnvValueMismatch, 0, len(migrationCompatibilityKeys))

	for _, key := range migrationCompatibilityKeys {
		leftValue := left.Value(key)
		rightValue := right.Value(key)
		if leftValue == rightValue {
			continue
		}
		mismatches = append(mismatches, EnvValueMismatch{
			Name:       key,
			LeftValue:  leftValue,
			RightValue: rightValue,
		})
	}

	return mismatches
}
