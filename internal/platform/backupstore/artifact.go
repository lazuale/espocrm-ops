package backupstore

import "fmt"

type ArtifactInspection struct {
	FileInfo         FileInfo
	SidecarPath      string
	SidecarInfo      FileInfo
	ChecksumVerified bool
	ChecksumError    error
}

func InspectBackupArtifact(path, label string, verifyChecksum bool) (ArtifactInspection, error) {
	fileInfo, err := InspectFile(path)
	if err != nil {
		return ArtifactInspection{}, err
	}

	sidecarPath := path + ".sha256"
	inspection := ArtifactInspection{
		FileInfo:    fileInfo,
		SidecarPath: sidecarPath,
	}
	if !fileInfo.Exists || fileInfo.IsDir {
		return inspection, nil
	}

	sidecarInfo, err := InspectFile(sidecarPath)
	if err != nil {
		return ArtifactInspection{}, err
	}
	inspection.SidecarInfo = sidecarInfo
	if !sidecarInfo.Exists {
		return inspection, nil
	}
	if sidecarInfo.IsDir {
		inspection.ChecksumError = ChecksumSidecarFormatError{
			Label: label,
			Path:  sidecarPath,
			Err:   fmt.Errorf("sidecar path is a directory"),
		}
		return inspection, nil
	}
	if !verifyChecksum {
		return inspection, nil
	}

	ok, err := verifySHA256Sidecar(label, path, sidecarPath)
	if err != nil {
		inspection.ChecksumError = err
		return inspection, nil
	}
	if ok {
		inspection.ChecksumVerified = true
		return inspection, nil
	}

	inspection.ChecksumError = ChecksumMismatchError{Label: label, Path: path}
	return inspection, nil
}
