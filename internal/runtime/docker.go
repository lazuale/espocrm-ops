package runtime

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

type Target struct {
	ProjectDir     string
	ComposeFile    string
	EnvFile        string
	DBService      string
	DBUser         string
	DBPassword     string
	DBRootPassword string
	DBName         string
}

func RequireCommands(names ...string) error {
	for _, name := range names {
		if _, err := exec.LookPath(name); err != nil {
			return fmt.Errorf("%s is required: %w", name, err)
		}
	}
	return nil
}

func ComposeConfig(ctx context.Context, target Target) error {
	return runCompose(ctx, target, nil, "config")
}

func StopServices(ctx context.Context, target Target, services []string) error {
	return runCompose(ctx, target, nil, append([]string{"stop"}, cleanServices(services)...)...)
}

func StartServices(ctx context.Context, target Target, services []string) error {
	return runCompose(ctx, target, nil, append([]string{"up", "-d"}, cleanServices(services)...)...)
}

func DumpDatabase(ctx context.Context, target Target, destPath string) error {
	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	dump := composeCommand(ctx, target, []string{"MYSQL_PWD=" + target.DBPassword},
		"exec", "-T", "-e", "MYSQL_PWD", target.DBService,
		"mariadb-dump",
		"--single-transaction",
		"--quick",
		"--routines",
		"--triggers",
		"--events",
		"--hex-blob",
		"--default-character-set=utf8mb4",
		"-u", target.DBUser,
		target.DBName,
	)
	gzip := exec.CommandContext(ctx, "gzip", "-c")
	return pipeCommands(dump, gzip, nil, out)
}

func ResetDatabase(ctx context.Context, target Target) error {
	return runCompose(ctx, target, []string{"MYSQL_PWD=" + target.DBRootPassword},
		"exec", "-T", "-e", "MYSQL_PWD", target.DBService,
		"mariadb",
		"-u", "root",
		"-e", resetDatabaseSQL(target.DBName),
	)
}

func RestoreDatabase(ctx context.Context, target Target, sourcePath string) error {
	gzip := exec.CommandContext(ctx, "gzip", "-dc", sourcePath)
	mysql := composeCommand(ctx, target, []string{"MYSQL_PWD=" + target.DBPassword},
		"exec", "-T", "-e", "MYSQL_PWD", target.DBService,
		"mariadb",
		"-u", target.DBUser,
		target.DBName,
	)
	return pipeCommands(gzip, mysql, nil, nil)
}

func DBPing(ctx context.Context, target Target) error {
	return runCompose(ctx, target, []string{"MYSQL_PWD=" + target.DBPassword},
		"exec", "-T", "-e", "MYSQL_PWD", target.DBService,
		"mariadb",
		"-u", target.DBUser,
		target.DBName,
		"-e", "SELECT 1;",
	)
}

func CreateStorageArchive(ctx context.Context, sourceDir, destPath string) error {
	return runCommand(ctx, "", nil, nil, nil, "tar", "-C", sourceDir, "-czf", destPath, ".")
}

func ExtractStorageArchive(ctx context.Context, archivePath, destDir string) error {
	return runCommand(ctx, "", nil, nil, nil, "tar", "-xzf", archivePath, "-C", destDir)
}

func TestGzip(ctx context.Context, path string) error {
	return runCommand(ctx, "", nil, nil, nil, "gzip", "-t", path)
}

func SHA256(ctx context.Context, path string) (string, error) {
	var stdout bytes.Buffer
	if err := runCommand(ctx, "", nil, nil, &stdout, "sha256sum", path); err != nil {
		return "", err
	}
	fields := strings.Fields(stdout.String())
	if len(fields) == 0 {
		return "", fmt.Errorf("sha256sum returned no checksum for %s", path)
	}
	return strings.ToLower(fields[0]), nil
}

func runCompose(ctx context.Context, target Target, env []string, args ...string) error {
	cmd := composeCommand(ctx, target, env, args...)
	return runPreparedCommand(cmd, nil, nil)
}

func composeCommand(ctx context.Context, target Target, env []string, args ...string) *exec.Cmd {
	cmdArgs := append([]string{
		"compose",
		"--env-file", strings.TrimSpace(target.EnvFile),
		"-f", strings.TrimSpace(target.ComposeFile),
	}, args...)
	cmd := exec.CommandContext(ctx, "docker", cmdArgs...)
	cmd.Dir = strings.TrimSpace(target.ProjectDir)
	cmd.Env = append(os.Environ(), env...)
	return cmd
}

func runCommand(ctx context.Context, dir string, env []string, stdin io.Reader, stdout io.Writer, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	return runPreparedCommand(cmd, stdin, stdout)
}

func runPreparedCommand(cmd *exec.Cmd, stdin io.Reader, stdout io.Writer) error {
	var stderr bytes.Buffer
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return commandError(cmd, stderr.String(), err)
	}
	return nil
}

func pipeCommands(first, second *exec.Cmd, stdin io.Reader, stdout io.Writer) error {
	reader, writer := io.Pipe()
	var firstErr, secondErr bytes.Buffer

	first.Stdin = stdin
	first.Stdout = writer
	first.Stderr = &firstErr
	second.Stdin = reader
	second.Stdout = stdout
	second.Stderr = &secondErr

	if err := second.Start(); err != nil {
		_ = reader.Close()
		_ = writer.Close()
		return commandError(second, secondErr.String(), err)
	}
	if err := first.Start(); err != nil {
		_ = writer.CloseWithError(err)
		_ = reader.Close()
		_ = second.Wait()
		return commandError(first, firstErr.String(), err)
	}

	err1 := first.Wait()
	if err1 != nil {
		_ = writer.CloseWithError(err1)
	} else {
		_ = writer.Close()
	}
	err2 := second.Wait()

	if err1 != nil {
		return commandError(first, firstErr.String(), err1)
	}
	if err2 != nil {
		return commandError(second, secondErr.String(), err2)
	}
	return nil
}

func commandError(cmd *exec.Cmd, stderr string, err error) error {
	message := strings.TrimSpace(stderr)
	if message == "" {
		return fmt.Errorf("%s: %w", strings.Join(cmd.Args, " "), err)
	}
	return fmt.Errorf("%s: %s: %w", strings.Join(cmd.Args, " "), message, err)
}

func cleanServices(services []string) []string {
	out := make([]string, 0, len(services))
	for _, service := range services {
		service = strings.TrimSpace(service)
		if service != "" {
			out = append(out, service)
		}
	}
	return out
}

func resetDatabaseSQL(name string) string {
	quoted := "`" + strings.ReplaceAll(name, "`", "``") + "`"
	return fmt.Sprintf("DROP DATABASE IF EXISTS %s; CREATE DATABASE %s CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;", quoted, quoted)
}
