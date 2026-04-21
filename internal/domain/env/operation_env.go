package env

type OperationEnv struct {
	FilePath        string
	ResolvedContour string
	Values          map[string]string
}

func (e OperationEnv) Value(key string) string {
	if e.Values == nil {
		return ""
	}

	return e.Values[key]
}

func (e OperationEnv) ComposeProject() string {
	return e.Value("COMPOSE_PROJECT_NAME")
}

func (e OperationEnv) DBStorageDir() string {
	return e.Value("DB_STORAGE_DIR")
}

func (e OperationEnv) ESPOStorageDir() string {
	return e.Value("ESPO_STORAGE_DIR")
}

func (e OperationEnv) BackupRoot() string {
	return e.Value("BACKUP_ROOT")
}
