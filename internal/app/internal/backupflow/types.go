package backupflow

import (
	"time"

	domainworkflow "github.com/lazuale/espocrm-ops/internal/domain/workflow"
)

type Options struct {
	ComposeFile string
	SkipDB      bool
	SkipFiles   bool
	NoStop      bool
	Now         func() time.Time
}

type Request struct {
	Scope          string
	ProjectDir     string
	ComposeFile    string
	EnvFile        string
	BackupRoot     string
	StorageDir     string
	NamePrefix     string
	RetentionDays  int
	ComposeProject string
	DBUser         string
	DBPassword     string
	DBName         string
	EspoCRMImage   string
	HelperImage    string
	MariaDBTag     string
	SkipDB         bool
	SkipFiles      bool
	NoStop         bool
	Now            func() time.Time
}

type ExecuteInfo struct {
	Scope                  string
	ProjectDir             string
	ComposeFile            string
	EnvFile                string
	BackupRoot             string
	CreatedAt              string
	RetentionDays          int
	ConsistentSnapshot     bool
	AppServicesWereRunning bool
	DBBackupCreated        bool
	FilesBackupCreated     bool
	SkipDB                 bool
	SkipFiles              bool
	NoStop                 bool
	ManifestTXTPath        string
	ManifestJSONPath       string
	DBBackupPath           string
	FilesBackupPath        string
	DBSidecarPath          string
	FilesSidecarPath       string
	Warnings               []string
	Steps                  []domainworkflow.Step
}
