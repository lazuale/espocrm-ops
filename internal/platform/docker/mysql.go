package docker

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"strings"
)

func RestoreMySQLDumpGz(dbPath, container, user, password, dbName string) error {
	f, err := os.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open db backup: %w", err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("open gzip reader: %w", err)
	}
	defer gz.Close()

	client, err := DetectDBClient(container)
	if err != nil {
		return err
	}

	if _, err := runCommand(commandOptions{
		Stdin: gz,
		Env:   dockerCommandEnv("MYSQL_PWD=" + password),
	},
		"docker", "exec", "-i",
		"-e", "MYSQL_PWD",
		container,
		client,
		"-u", user,
		dbName,
	); err != nil {
		return DBExecutionError{
			Action:    "restore mysql dump",
			Container: container,
			Err:       err,
		}
	}

	return nil
}

func ResetAndRestoreMySQLDumpGz(dbPath, container, rootPassword, dbName, appUser string) error {
	client, err := DetectDBClient(container)
	if err != nil {
		return err
	}

	resetSQL := fmt.Sprintf(
		"DROP DATABASE IF EXISTS `%s`;\nCREATE DATABASE `%s` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;\nGRANT ALL PRIVILEGES ON `%s`.* TO '%s'@'%%';\nFLUSH PRIVILEGES;\n",
		dbName,
		dbName,
		dbName,
		appUser,
	)
	if err := runMySQLSQL(container, client, rootPassword, "", resetSQL); err != nil {
		return DBExecutionError{
			Action:    fmt.Sprintf("reset mysql database %s", dbName),
			Container: container,
			Err:       err,
		}
	}

	f, err := os.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open db backup: %w", err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("open gzip reader: %w", err)
	}
	defer gz.Close()

	if err := pipeMySQL(container, client, rootPassword, dbName, gz); err != nil {
		return DBExecutionError{
			Action:    "restore mysql dump",
			Container: container,
			Err:       err,
		}
	}

	return nil
}

func DetectDBClient(container string) (string, error) {
	var lastErr error
	for _, client := range []string{"mariadb", "mysql"} {
		if _, err := Run("docker", "exec", container, client, "--version"); err == nil {
			return client, nil
		} else {
			lastErr = err
		}
	}

	return "", DBClientDetectionError{Container: container, Err: lastErr}
}

func runMySQLSQL(container, client, password, dbName, sql string) error {
	return pipeMySQL(container, client, password, dbName, strings.NewReader(sql))
}

func pipeMySQL(container, client, password, dbName string, input io.Reader) error {
	args := []string{
		"exec", "-i",
		"-e", "MYSQL_PWD",
		container,
		client,
		"-u", "root",
	}
	if dbName != "" {
		args = append(args, dbName)
	}

	if _, err := runCommand(commandOptions{
		Stdin: input,
		Env:   dockerCommandEnv("MYSQL_PWD=" + password),
	}, "docker", args...); err != nil {
		return err
	}

	return nil
}
