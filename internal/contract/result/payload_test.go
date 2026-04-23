package result

var (
	_ DetailsPayload = BackupVerifyDetails{}
	_ DetailsPayload = BackupDetails{}
	_ DetailsPayload = RestoreDetails{}
	_ DetailsPayload = MigrateDetails{}
	_ DetailsPayload = DoctorDetails{}

	_ ArtifactsPayload = BackupVerifyArtifacts{}
	_ ArtifactsPayload = BackupArtifacts{}
	_ ArtifactsPayload = RestoreArtifacts{}
	_ ArtifactsPayload = MigrateArtifacts{}
	_ ArtifactsPayload = DoctorArtifacts{}

	_ ItemPayload = BackupItem{}
	_ ItemPayload = BackupVerifyItem{}
	_ ItemPayload = RestoreItem{}
	_ ItemPayload = MigrateItem{}
	_ ItemPayload = DoctorCheck{}
)
