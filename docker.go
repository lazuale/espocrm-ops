package main

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"os/exec"
)

func dockerComposeArgs(password string, serviceArgs ...string) []string {
	args := []string{"compose", "--env-file", ".env", "-f", "compose.yaml", "exec", "-T", "-e", "MYSQL_PWD=" + password, "db"}
	return append(args, serviceArgs...)
}

func dumpDatabase(cfg Config, out io.Writer) error {
	args := dockerComposeArgs(
		cfg.DBPassword,
		"mariadb-dump",
		"--single-transaction",
		"--quick",
		"--routines",
		"--triggers",
		"--events",
		"--hex-blob",
		"--default-character-set=utf8mb4",
		"-u", cfg.DBUser,
		cfg.DBName,
	)
	cmd := exec.Command("docker", args...)
	cmd.Stderr = os.Stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("open database dump stream: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start database dump: %w", err)
	}

	gz := gzip.NewWriter(out)
	copyErr := copyAndCloseGzip(gz, stdout)
	waitErr := cmd.Wait()
	if copyErr != nil {
		return copyErr
	}
	if waitErr != nil {
		return fmt.Errorf("database dump failed: %w", waitErr)
	}
	return nil
}

func resetDatabase(cfg Config) error {
	sql := fmt.Sprintf("DROP DATABASE IF EXISTS `%s`; CREATE DATABASE `%s` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;", cfg.DBName, cfg.DBName)
	args := dockerComposeArgs(cfg.DBRootPassword, "mariadb", "-u", "root", "-e", sql)
	cmd := exec.Command("docker", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("reset database failed: %w", err)
	}
	return nil
}

func restoreDatabase(cfg Config, gzSQL io.Reader) error {
	gz, err := gzip.NewReader(gzSQL)
	if err != nil {
		return fmt.Errorf("open db dump gzip: %w", err)
	}
	defer gz.Close()

	args := dockerComposeArgs(cfg.DBPassword, "mariadb", "-u", cfg.DBUser, cfg.DBName)
	cmd := exec.Command("docker", args...)
	cmd.Stdin = gz
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("restore database failed: %w", err)
	}
	return nil
}

func copyAndCloseGzip(gz *gzip.Writer, r io.Reader) error {
	if _, err := io.Copy(gz, r); err != nil {
		gz.Close()
		return fmt.Errorf("write compressed stream: %w", err)
	}
	if err := gz.Close(); err != nil {
		return fmt.Errorf("close compressed stream: %w", err)
	}
	return nil
}
