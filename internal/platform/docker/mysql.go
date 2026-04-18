package docker

import (
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

func DumpMySQLDumpGz(cfg ComposeConfig, service, user, password, dbName, destPath string) (err error) {
	container, err := composeServiceContainerID(cfg, service)
	if err != nil {
		return fmt.Errorf("resolve db container for service %s: %w", service, err)
	}
	if strings.TrimSpace(container) == "" {
		return ContainerNotRunningError{Container: service}
	}

	dumpClient, err := detectDBDumpClient(container)
	if err != nil {
		return err
	}

	f, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create db backup: %w", err)
	}
	defer closeMySQLResource(f, fmt.Sprintf("db backup file %s", destPath), &err)

	gz, err := gzip.NewWriterLevel(f, gzip.BestCompression)
	if err != nil {
		return fmt.Errorf("create gzip writer: %w", err)
	}
	closed := false
	defer func() {
		if !closed {
			closeMySQLResource(gz, fmt.Sprintf("db backup gzip writer for %s", destPath), &err)
		}
	}()

	var stderr strings.Builder
	cmd := exec.Command(
		"docker",
		"exec", "-i",
		"-e", "MYSQL_PWD",
		container,
		dumpClient,
		"-u", user,
		dbName,
		"--single-transaction",
		"--quick",
		"--routines",
		"--triggers",
		"--events",
	)
	cmd.Env = dockerCommandEnv("MYSQL_PWD=" + password)
	cmd.Stdout = gz
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("dump mysql database %s in container %s: %w%s", dbName, container, err, commandErrorSuffix(stderr.String()))
	}
	if err := gz.Close(); err != nil {
		return fmt.Errorf("finish db backup gzip: %w", err)
	}
	closed = true

	return nil
}

func RestoreMySQLDumpGz(dbPath, container, user, password, dbName string) (err error) {
	f, err := os.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open db backup: %w", err)
	}
	defer closeMySQLResource(f, fmt.Sprintf("db backup file %s", dbPath), &err)

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("open gzip reader: %w", err)
	}
	defer closeMySQLResource(gz, fmt.Sprintf("db backup gzip reader for %s", dbPath), &err)

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

func ResetAndRestoreMySQLDumpGz(dbPath, container, rootPassword, dbName, appUser string) (err error) {
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
	defer closeMySQLResource(f, fmt.Sprintf("db backup file %s", dbPath), &err)

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("open gzip reader: %w", err)
	}
	defer closeMySQLResource(gz, fmt.Sprintf("db backup gzip reader for %s", dbPath), &err)

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

func detectDBDumpClient(container string) (string, error) {
	var lastErr error
	for _, client := range []string{"mariadb-dump", "mysqldump"} {
		if _, err := Run("docker", "exec", container, client, "--version"); err == nil {
			return client, nil
		} else {
			lastErr = err
		}
	}

	return "", fmt.Errorf("detect db dump client in container %s: %w", container, lastErr)
}

func closeMySQLResource(closer interface{ Close() error }, label string, errp *error) {
	if closer == nil {
		return
	}

	if closeErr := closer.Close(); closeErr != nil {
		wrapped := fmt.Errorf("close %s: %w", label, closeErr)
		if *errp == nil {
			*errp = wrapped
		} else {
			*errp = errors.Join(*errp, wrapped)
		}
	}
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

func commandErrorSuffix(stderr string) string {
	if line := lastNonBlankLine(stderr); line != "" {
		return ": " + line
	}

	return ""
}

func lastNonBlankLine(text string) string {
	text = strings.ReplaceAll(text, "\r", "")
	lines := strings.Split(text, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			return line
		}
	}

	return ""
}
