package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

type Config struct {
	BackupRoot     string
	EspoStorageDir string
	DBUser         string
	DBPassword     string
	DBRootPassword string
	DBName         string
}

var dbNameRE = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

func LoadConfig(path string) (Config, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Config{}, fmt.Errorf("missing .env file")
		}
		return Config{}, fmt.Errorf("open .env: %w", err)
	}
	defer file.Close()

	values := map[string]string{}
	scanner := bufio.NewScanner(file)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return Config{}, fmt.Errorf(".env line %d: expected KEY=VALUE", lineNo)
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" {
			return Config{}, fmt.Errorf(".env line %d: empty key", lineNo)
		}
		if _, exists := values[key]; exists {
			return Config{}, fmt.Errorf(".env line %d: duplicate key %s", lineNo, key)
		}
		values[key] = value
	}
	if err := scanner.Err(); err != nil {
		return Config{}, fmt.Errorf("read .env: %w", err)
	}

	required := []string{"BACKUP_ROOT", "ESPO_STORAGE_DIR", "DB_USER", "DB_PASSWORD", "DB_ROOT_PASSWORD", "DB_NAME"}
	for _, key := range required {
		value, ok := values[key]
		if !ok {
			return Config{}, fmt.Errorf(".env missing required key %s", key)
		}
		if value == "" {
			return Config{}, fmt.Errorf(".env key %s must not be empty", key)
		}
	}

	if err := validateDBName(values["DB_NAME"]); err != nil {
		return Config{}, err
	}
	if err := validateDBUser(values["DB_USER"]); err != nil {
		return Config{}, err
	}

	storage, err := validateStoragePath(values["ESPO_STORAGE_DIR"])
	if err != nil {
		return Config{}, err
	}
	backupRoot, err := validateBackupRootPath(values["BACKUP_ROOT"], storage)
	if err != nil {
		return Config{}, err
	}

	return Config{
		BackupRoot:     backupRoot,
		EspoStorageDir: storage,
		DBUser:         values["DB_USER"],
		DBPassword:     values["DB_PASSWORD"],
		DBRootPassword: values["DB_ROOT_PASSWORD"],
		DBName:         values["DB_NAME"],
	}, nil
}

func validateDBName(name string) error {
	if !dbNameRE.MatchString(name) {
		return fmt.Errorf("DB_NAME must match [A-Za-z0-9_]+")
	}
	return nil
}

func validateDBUser(user string) error {
	if user == "" {
		return fmt.Errorf("DB_USER must not be empty")
	}
	for _, r := range user {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return fmt.Errorf("DB_USER must not contain whitespace or control characters")
		}
	}
	return nil
}

func validateStoragePath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("ESPO_STORAGE_DIR must not be empty")
	}
	if strings.ContainsRune(path, 0) {
		return "", fmt.Errorf("ESPO_STORAGE_DIR must not contain NUL")
	}
	if path == "/" {
		return "", fmt.Errorf("ESPO_STORAGE_DIR must not be filesystem root")
	}
	if path == "." {
		return "", fmt.Errorf("ESPO_STORAGE_DIR must not be .")
	}
	abs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", fmt.Errorf("resolve ESPO_STORAGE_DIR: %w", err)
	}
	if isRootPath(abs) {
		return "", fmt.Errorf("ESPO_STORAGE_DIR must not resolve to filesystem root")
	}
	home, err := os.UserHomeDir()
	if err == nil {
		homeAbs, err := filepath.Abs(filepath.Clean(home))
		if err == nil && abs == homeAbs {
			return "", fmt.Errorf("ESPO_STORAGE_DIR must not resolve to user home")
		}
	}
	return abs, nil
}

func validateBackupRootPath(path string, storage string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("BACKUP_ROOT must not be empty")
	}
	if strings.ContainsRune(path, 0) {
		return "", fmt.Errorf("BACKUP_ROOT must not contain NUL")
	}
	if path == "/" {
		return "", fmt.Errorf("BACKUP_ROOT must not be filesystem root")
	}
	abs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", fmt.Errorf("resolve BACKUP_ROOT: %w", err)
	}
	if isRootPath(abs) {
		return "", fmt.Errorf("BACKUP_ROOT must not resolve to filesystem root")
	}
	rel, err := filepath.Rel(storage, abs)
	if err != nil {
		return "", fmt.Errorf("compare BACKUP_ROOT and ESPO_STORAGE_DIR: %w", err)
	}
	if rel == "." {
		return "", fmt.Errorf("BACKUP_ROOT must not equal ESPO_STORAGE_DIR")
	}
	if rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("BACKUP_ROOT must not be inside ESPO_STORAGE_DIR")
	}
	return abs, nil
}

func isRootPath(path string) bool {
	clean := filepath.Clean(path)
	return filepath.Dir(clean) == clean
}
