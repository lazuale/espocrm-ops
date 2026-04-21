package docker

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateTarArchiveViaHelperUsesTarEntrypointAndFilteredEnv(t *testing.T) {
	tmp := t.TempDir()
	sourceDir := filepath.Join(tmp, "source")
	childDir := filepath.Join(sourceDir, "data")
	archivePath := filepath.Join(tmp, "files.tar.gz")
	logPath := filepath.Join(tmp, "docker.log")
	envLogPath := filepath.Join(tmp, "docker.env")

	if err := os.MkdirAll(childDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(childDir, "test.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("UNRELATED_SECRET", "host-only-secret")
	prependFakeDocker(t, fakeDockerOptions{
		logPath:         logPath,
		envLogPath:      envLogPath,
		availableImages: []string{"espocrm/espocrm:9.3.4-apache"},
	})

	if err := CreateTarArchiveViaHelper(sourceDir, archivePath, "10.11", "espocrm/espocrm:9.3.4-apache"); err != nil {
		t.Fatalf("CreateTarArchiveViaHelper failed: %v", err)
	}

	rawLog, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	log := string(rawLog)
	if !strings.Contains(log, "run --pull=never --rm --entrypoint tar") {
		t.Fatalf("expected tar entrypoint docker run, got %s", log)
	}
	if strings.Contains(log, " --entrypoint sh ") || strings.Contains(log, " -euc ") {
		t.Fatalf("archive helper should not go through shell: %s", log)
	}

	rawEnv, err := os.ReadFile(envLogPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(rawEnv), "UNRELATED_SECRET=host-only-secret") {
		t.Fatalf("unexpected host env leak into archive helper: %s", string(rawEnv))
	}

	archiveEntries := readArchiveEntryBodies(t, archivePath)
	if got := archiveEntries[filepath.Base(sourceDir)+"/data/test.txt"]; got != "hello\n" {
		t.Fatalf("unexpected archive body: %q", got)
	}
}

func TestSelectLocalHelperImagePrefersRuntimeImagesBeforeBuiltinFallback(t *testing.T) {
	tmp := t.TempDir()
	logPath := filepath.Join(tmp, "docker.log")

	prependFakeDocker(t, fakeDockerOptions{
		logPath:         logPath,
		availableImages: []string{"mariadb:10.11", "alpine:3.20"},
	})

	image, err := selectLocalHelperImage("10.11", "espocrm/custom:1.0")
	if err != nil {
		t.Fatalf("selectLocalHelperImage failed: %v", err)
	}
	if image != "mariadb:10.11" {
		t.Fatalf("expected mariadb runtime image, got %s", image)
	}

	rawLog, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(rawLog)), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least two image inspect probes, got %q", string(rawLog))
	}
	if !strings.Contains(lines[0], "image inspect espocrm/custom:1.0") {
		t.Fatalf("expected espocrm image to be probed first, got %q", lines[0])
	}
	if !strings.Contains(lines[1], "image inspect mariadb:10.11") {
		t.Fatalf("expected mariadb image to be probed second, got %q", lines[1])
	}
}

func TestSelectLocalHelperImageDoesNotHideUnexpectedInspectFailure(t *testing.T) {
	prependFakeDocker(t, fakeDockerOptions{
		imageInspectStderr:   "permission denied",
		imageInspectExitCode: 23,
	})

	_, err := selectLocalHelperImage("10.11", "espocrm/espocrm:9.3.4-apache")
	if err == nil {
		t.Fatal("expected helper image inspect error")
	}
	if !strings.Contains(err.Error(), "inspect helper image") {
		t.Fatalf("expected explicit inspect failure, got %v", err)
	}
	if !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("expected inspect stderr in error, got %v", err)
	}
}

func readArchiveEntryBodies(t *testing.T, archivePath string) map[string]string {
	t.Helper()

	f, err := os.Open(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			t.Fatalf("close archive file: %v", closeErr)
		}
	}()

	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if closeErr := gz.Close(); closeErr != nil {
			t.Fatalf("close archive gzip reader: %v", closeErr)
		}
	}()

	tr := tar.NewReader(gz)
	entries := map[string]string{}
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return entries
		}
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(tr)
		if err != nil {
			t.Fatal(err)
		}
		entries[hdr.Name] = string(body)
	}
}
