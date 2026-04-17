package config

import (
	"os"
	"strings"
)

func ResolveDBPassword(cfg DBConfig) (string, error) {
	return resolvePassword(cfg.Password, cfg.PasswordFile, "db password")
}

func ResolveDBRootPassword(cfg DBConfig) (string, error) {
	return resolvePassword(cfg.Password, cfg.PasswordFile, "db root password")
}

func resolvePassword(password, passwordFile string, label string) (string, error) {
	if strings.TrimSpace(password) != "" && strings.TrimSpace(passwordFile) != "" {
		return "", PasswordSourceConflictError{Label: label}
	}

	if strings.TrimSpace(password) != "" {
		return password, nil
	}

	if strings.TrimSpace(passwordFile) != "" {
		raw, err := os.ReadFile(passwordFile)
		if err != nil {
			return "", PasswordFileReadError{Path: passwordFile, Err: err}
		}
		pw := strings.TrimSpace(string(raw))
		if pw == "" {
			return "", PasswordFileEmptyError{Path: passwordFile}
		}
		return pw, nil
	}

	return "", PasswordRequiredError{Label: label}
}
