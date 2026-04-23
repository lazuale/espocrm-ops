package result

import (
	"reflect"
	"strings"
	"testing"
)

func TestMigrateResultContractDoesNotExposeLegacySelectionFields(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value any
	}{
		{name: "details", value: MigrateDetails{}},
		{name: "artifacts", value: MigrateArtifacts{}},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			typ := reflect.TypeOf(tc.value)
			for i := 0; i < typ.NumField(); i++ {
				field := typ.Field(i)
				fieldName := field.Name
				jsonName := strings.Split(field.Tag.Get("json"), ",")[0]
				for _, forbidden := range []string{
					"RequestedSelectionMode",
					"RequestedDBBackup",
					"RequestedFilesBackup",
					"SelectedPrefix",
					"SelectedStamp",
					"requested_selection_mode",
					"requested_db_backup",
					"requested_files_backup",
					"selected_prefix",
					"selected_stamp",
				} {
					if fieldName == forbidden || jsonName == forbidden {
						t.Fatalf("migrate %s contract exposes legacy selection field %q", tc.name, forbidden)
					}
				}
			}
		})
	}
}
