package backup

type VerifyRequest struct {
	ManifestPath string
}

type VerifyInfo struct {
	ManifestPath string
	Scope        string
	CreatedAt    string
	DBBackupPath string
	FilesPath    string
}

func (s Service) Verify(req VerifyRequest) error {
	return s.store.VerifyManifest(req.ManifestPath)
}

func (s Service) VerifyDetailed(req VerifyRequest) (VerifyInfo, error) {
	info, err := s.store.VerifyManifestDetailed(req.ManifestPath)
	if err != nil {
		return VerifyInfo{}, err
	}

	return VerifyInfo{
		ManifestPath: info.ManifestPath,
		Scope:        info.Scope,
		CreatedAt:    info.CreatedAt,
		DBBackupPath: info.DBBackupPath,
		FilesPath:    info.FilesPath,
	}, nil
}
