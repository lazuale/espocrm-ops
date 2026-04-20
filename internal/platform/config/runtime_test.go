package config

import "testing"

func TestMigrationCompatibilityMismatches(t *testing.T) {
	left := OperationEnv{
		Values: map[string]string{
			"ESPOCRM_IMAGE":         "espocrm/espocrm:9.3.4-apache",
			"MARIADB_TAG":           "10.11",
			"ESPO_DEFAULT_LANGUAGE": "en_US",
			"ESPO_TIME_ZONE":        "UTC",
		},
	}
	right := OperationEnv{
		Values: map[string]string{
			"ESPOCRM_IMAGE":         "espocrm/espocrm:9.3.5-apache",
			"MARIADB_TAG":           "10.11",
			"ESPO_DEFAULT_LANGUAGE": "ru_RU",
			"ESPO_TIME_ZONE":        "Europe/Moscow",
		},
	}

	mismatches := MigrationCompatibilityMismatches(left, right)
	if len(mismatches) != 3 {
		t.Fatalf("expected 3 mismatches, got %#v", mismatches)
	}
	if mismatches[0].Name != "ESPOCRM_IMAGE" {
		t.Fatalf("unexpected first mismatch: %#v", mismatches[0])
	}
	if mismatches[1].Name != "ESPO_DEFAULT_LANGUAGE" {
		t.Fatalf("unexpected second mismatch: %#v", mismatches[1])
	}
	if mismatches[2].Name != "ESPO_TIME_ZONE" {
		t.Fatalf("unexpected third mismatch: %#v", mismatches[2])
	}
}
