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
	helperImage := "registry.example.com/espops-helper:1.0"

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
		availableImages: []string{helperImage},
	})

	if err := CreateTarArchiveViaHelper(sourceDir, archivePath, helperImage); err != nil {
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
	if !strings.Contains(log, "image inspect "+helperImage) {
		t.Fatalf("expected explicit helper image probe, got %s", log)
	}
	if strings.Contains(log, "image inspect mariadb:") || strings.Contains(log, "image inspect alpine:") || strings.Contains(log, "image inspect busybox:") {
		t.Fatalf("archive helper must not probe fallback images, got %s", log)
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

func TestCreateTarArchiveViaHelperFailsClosedWhenHelperImageIsMissing(t *testing.T) {
	err := CreateTarArchiveViaHelper(t.TempDir(), filepath.Join(t.TempDir(), "files.tar.gz"), "registry.example.com/espops-helper:1.0")
	if err == nil {
		t.Fatal("expected missing helper image error")
	}
	if !strings.Contains(err.Error(), "is not available locally") {
		t.Fatalf("unexpected missing helper image error: %v", err)
	}
}

func TestCreateTarArchiveViaHelperDoesNotHideUnexpectedInspectFailure(t *testing.T) {
	prependFakeDocker(t, fakeDockerOptions{
		imageInspectStderr:   "permission denied",
		imageInspectExitCode: 23,
	})

	err := CreateTarArchiveViaHelper(t.TempDir(), filepath.Join(t.TempDir(), "files.tar.gz"), "registry.example.com/espops-helper:1.0")
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
