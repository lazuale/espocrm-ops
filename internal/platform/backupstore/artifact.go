package backupstore

import "fmt"

type artifactInspection struct {
	FileInfo         fileInfo
	SidecarPath      string
	SidecarInfo      fileInfo
	ChecksumVerified bool
	ChecksumError    error
}

func inspectBackupArtifact(path, label string, verifyChecksum bool) (artifactInspection, error) {
	fileInfo, err := inspectFile(path)
	if err != nil {
		return artifactInspection{}, err
	}

	sidecarPath := path + ".sha256"
	inspection := artifactInspection{
		FileInfo:    fileInfo,
		SidecarPath: sidecarPath,
	}
	if !fileInfo.Exists || fileInfo.IsDir {
		return inspection, nil
	}

	sidecarInfo, err := inspectFile(sidecarPath)
	if err != nil {
		return artifactInspection{}, err
	}
	inspection.SidecarInfo = sidecarInfo
	if !sidecarInfo.Exists {
		return inspection, nil
	}
	if sidecarInfo.IsDir {
		inspection.ChecksumError = checksumSidecarFormatError{
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

	inspection.ChecksumError = checksumMismatchError{Label: label, Path: path}
	return inspection, nil
}
