package main

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestValidateBackupRejectsMismatchedHash(t *testing.T) {
	dir := createValidBackupFixture(t)
	if err := os.WriteFile(filepath.Join(dir, sumsFileName), []byte(strings.Repeat("0", 64)+"  "+dbFileName+"\n"+strings.Repeat("1", 64)+"  "+filesFileName+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := ValidateBackup(dir); err == nil {
		t.Fatal("expected mismatched hash rejected")
	}
}

func TestValidateBackupRejectsMissingManifest(t *testing.T) {
	dir := createValidBackupFixture(t)
	if err := os.Remove(filepath.Join(dir, manifestFileName)); err != nil {
		t.Fatal(err)
	}
	if err := ValidateBackup(dir); err == nil {
		t.Fatal("expected missing manifest rejected")
	}
}

func TestValidateBackupRejectsMissingSHA256SUMS(t *testing.T) {
	dir := createValidBackupFixture(t)
	if err := os.Remove(filepath.Join(dir, sumsFileName)); err != nil {
		t.Fatal(err)
	}
	if err := ValidateBackup(dir); err == nil {
		t.Fatal("expected missing SHA256SUMS rejected")
	}
}

func TestValidateBackupAcceptsValidFixture(t *testing.T) {
	dir := createValidBackupFixture(t)
	if err := ValidateBackup(dir); err != nil {
		t.Fatalf("expected valid backup accepted: %v", err)
	}
}

func createValidBackupFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, dbFileName)
	filesPath := filepath.Join(dir, filesFileName)
	writeGzipFile(t, dbPath, "CREATE TABLE test (id int);\n")
	writeNormalFilesArchive(t, filesPath)

	dbHash, dbSize, err := fileSHA256(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	filesHash, filesSize, err := fileSHA256(filesPath)
	if err != nil {
		t.Fatal(err)
	}
	manifest := newManifest(time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC), checksumFile{Hash: dbHash, Size: dbSize}, checksumFile{Hash: filesHash, Size: filesSize})
	if err := writeManifest(filepath.Join(dir, manifestFileName), manifest); err != nil {
		t.Fatal(err)
	}
	if err := writeSHA256SUMS(filepath.Join(dir, sumsFileName), dbHash, filesHash); err != nil {
		t.Fatal(err)
	}
	return dir
}

func writeGzipFile(t *testing.T, path string, body string) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(file)
	if _, err := gz.Write([]byte(body)); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func writeNormalFilesArchive(t *testing.T, path string) {
	t.Helper()
	temp := writeTestArchive(t, []testTarEntry{
		{Name: "storage/", Typeflag: tar.TypeDir},
		{Name: "storage/file.txt", Body: "hello"},
	})
	data, err := os.ReadFile(temp)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}
