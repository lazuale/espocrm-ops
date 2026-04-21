package restoreflow

import domainrestore "github.com/lazuale/espocrm-ops/internal/domain/restore"

type DBRequest struct {
	ManifestPath       string
	DBBackup           string
	DBContainer        string
	DBName             string
	DBUser             string
	DBPassword         string
	DBPasswordFile     string
	DBRootPassword     string
	DBRootPasswordFile string
	DryRun             bool
}

func (r DBRequest) Validate() error {
	return domainrestore.RestoreDBRequest{
		ManifestPath:       r.ManifestPath,
		DBBackup:           r.DBBackup,
		DBContainer:        r.DBContainer,
		DBName:             r.DBName,
		DBUser:             r.DBUser,
		DBPassword:         r.DBPassword,
		DBPasswordFile:     r.DBPasswordFile,
		DBRootPassword:     r.DBRootPassword,
		DBRootPasswordFile: r.DBRootPasswordFile,
		DryRun:             r.DryRun,
	}.Validate()
}

type FilesRequest struct {
	ManifestPath string
	FilesBackup  string
	TargetDir    string
	DryRun       bool
}

func (r FilesRequest) Validate() error {
	return domainrestore.RestoreFilesRequest{
		ManifestPath: r.ManifestPath,
		FilesBackup:  r.FilesBackup,
		TargetDir:    r.TargetDir,
		DryRun:       r.DryRun,
	}.Validate()
}

type FilesPreflightRequest struct {
	ManifestPath string
	FilesBackup  string
	TargetDir    string
}

func (r FilesPreflightRequest) Validate() error {
	return domainrestore.FilesPreflightRequest{
		ManifestPath: r.ManifestPath,
		FilesBackup:  r.FilesBackup,
		TargetDir:    r.TargetDir,
	}.Validate()
}

type DBPreflightRequest struct {
	ManifestPath string
	DBBackup     string
	DBContainer  string
}

func (r DBPreflightRequest) Validate() error {
	return domainrestore.DBPreflightRequest{
		ManifestPath: r.ManifestPath,
		DBBackup:     r.DBBackup,
		DBContainer:  r.DBContainer,
	}.Validate()
}
