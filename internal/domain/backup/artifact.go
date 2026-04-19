package backup

import (
	"fmt"
	"path/filepath"
	"regexp"
)

var (
	dbBackupNamePattern    = regexp.MustCompile(`^(.+)_([0-9]{4}-[0-9]{2}-[0-9]{2}_[0-9]{2}-[0-9]{2}-[0-9]{2})\.sql\.gz$`)
	filesBackupNamePattern = regexp.MustCompile(`^(.+)_files_([0-9]{4}-[0-9]{2}-[0-9]{2}_[0-9]{2}-[0-9]{2}-[0-9]{2})\.tar\.gz$`)
	manifestNamePattern    = regexp.MustCompile(`^(.+)_([0-9]{4}-[0-9]{2}-[0-9]{2}_[0-9]{2}-[0-9]{2}-[0-9]{2})\.manifest\.(txt|json)$`)
)

type ManifestKind string

const (
	ManifestTXT  ManifestKind = "txt"
	ManifestJSON ManifestKind = "json"
)

type BackupGroup struct {
	Prefix string
	Stamp  string
}

type BackupFile struct {
	Path string
}

type BackupSet struct {
	Group        BackupGroup
	DBBackup     BackupFile
	FilesBackup  BackupFile
	ManifestTXT  BackupFile
	ManifestJSON BackupFile
}

func BackupSetID(group BackupGroup) string {
	return fmt.Sprintf("%s_%s", group.Prefix, group.Stamp)
}

func (s BackupSet) ValidateSelection(skipDB, skipFiles bool) error {
	if skipDB && skipFiles {
		return fmt.Errorf("nothing selected: both db and files verification are skipped")
	}

	if !skipDB && s.DBBackup.Path == "" {
		return fmt.Errorf("db backup path is required")
	}

	if !skipFiles && s.FilesBackup.Path == "" {
		return fmt.Errorf("files backup path is required")
	}

	return nil
}

func BuildBackupSet(root, prefix, stamp string) BackupSet {
	return BackupSet{
		Group: BackupGroup{
			Prefix: prefix,
			Stamp:  stamp,
		},
		DBBackup: BackupFile{
			Path: filepath.Join(root, "db", fmt.Sprintf("%s_%s.sql.gz", prefix, stamp)),
		},
		FilesBackup: BackupFile{
			Path: filepath.Join(root, "files", fmt.Sprintf("%s_files_%s.tar.gz", prefix, stamp)),
		},
		ManifestTXT: BackupFile{
			Path: filepath.Join(root, "manifests", fmt.Sprintf("%s_%s.manifest.txt", prefix, stamp)),
		},
		ManifestJSON: BackupFile{
			Path: filepath.Join(root, "manifests", fmt.Sprintf("%s_%s.manifest.json", prefix, stamp)),
		},
	}
}

func ParseDBBackupName(path string) (BackupGroup, error) {
	return parseBackupGroup(filepath.Base(path), dbBackupNamePattern, "db backup")
}

func ParseFilesBackupName(path string) (BackupGroup, error) {
	return parseBackupGroup(filepath.Base(path), filesBackupNamePattern, "files backup")
}

func ParseManifestName(path string) (BackupGroup, ManifestKind, error) {
	match := manifestNamePattern.FindStringSubmatch(filepath.Base(path))
	if match == nil {
		return BackupGroup{}, "", fmt.Errorf("invalid manifest name: %s", filepath.Base(path))
	}

	return BackupGroup{
		Prefix: match[1],
		Stamp:  match[2],
	}, ManifestKind(match[3]), nil
}

func ResolveManifestArtifactPath(manifestPath, kind, fileName string) string {
	manifestDir := filepath.Dir(manifestPath)
	if filepath.Base(manifestDir) == "manifests" {
		return filepath.Join(filepath.Dir(manifestDir), kind, fileName)
	}

	return filepath.Join(manifestDir, fileName)
}

func parseBackupGroup(name string, pattern *regexp.Regexp, label string) (BackupGroup, error) {
	match := pattern.FindStringSubmatch(name)
	if match == nil {
		return BackupGroup{}, fmt.Errorf("invalid %s name: %s", label, name)
	}

	return BackupGroup{
		Prefix: match[1],
		Stamp:  match[2],
	}, nil
}
