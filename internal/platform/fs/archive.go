package fs

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"
)

const legacyTarRegularType = byte(0)

func VerifyGzipReadable(filePath string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()

	_, err = io.Copy(io.Discard, gz)
	return err
}

func VerifyTarGzReadable(filePath string, validateHeader func(*tar.Header) error) error {
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	var found bool

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		if validateHeader != nil {
			if err := validateHeader(hdr); err != nil {
				return err
			}
		}

		cleanName := pathpkg.Clean(hdr.Name)
		if cleanName != "." && cleanName != "/" {
			found = true
		}

		if _, err := io.Copy(io.Discard, tr); err != nil {
			return err
		}
	}

	if !found {
		return fmt.Errorf("tar archive is empty")
	}

	return nil
}

func UnpackTarGz(archivePath, destDir string, validateHeader func(*tar.Header) error) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return ArchiveReadError{Path: archivePath, Err: err}
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	found := false

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return ArchiveReadError{Path: archivePath, Err: err}
		}

		if validateHeader != nil {
			if err := validateHeader(hdr); err != nil {
				return err
			}
		}

		name := pathpkg.Clean(hdr.Name)
		if name == "." || name == "/" {
			continue
		}

		targetPath := filepath.Join(destDir, filepath.FromSlash(name))
		rel, err := filepath.Rel(destDir, targetPath)
		if err != nil {
			return err
		}
		if rel == ".." || hasFilesystemDotDotPrefix(rel) {
			return ArchiveEntryEscapeError{ArchivePath: archivePath, EntryName: hdr.Name}
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := ensureArchiveDirTarget(destDir, archivePath, hdr.Name, targetPath); err != nil {
				return err
			}
			if err := os.MkdirAll(targetPath, 0o755); err != nil {
				return err
			}
		case tar.TypeReg, legacyTarRegularType:
			if err := ensureArchiveFileTarget(destDir, archivePath, hdr.Name, targetPath); err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				_ = out.Close()
				return err
			}
			if err := out.Close(); err != nil {
				return err
			}
		default:
			return ArchiveUnexpectedEntryTypeError{ArchivePath: archivePath, EntryName: hdr.Name, Typeflag: hdr.Typeflag}
		}

		found = true
	}

	if !found {
		return ArchiveEmptyError{ArchivePath: archivePath}
	}

	return nil
}

func hasFilesystemDotDotPrefix(p string) bool {
	p = filepath.Clean(p)
	return p == ".." || strings.HasPrefix(p, ".."+string(os.PathSeparator))
}

func ensureArchiveDirTarget(destDir, archivePath, entryName, targetPath string) error {
	conflictPath, err := firstNonDirPath(destDir, targetPath)
	if err != nil {
		return err
	}
	if conflictPath != "" {
		return ArchiveEntryConflictError{
			ArchivePath:  archivePath,
			EntryName:    entryName,
			ConflictPath: conflictPath,
			Reason:       "directory path resolves through a file",
		}
	}

	return nil
}

func ensureArchiveFileTarget(destDir, archivePath, entryName, targetPath string) error {
	parent := filepath.Dir(targetPath)
	conflictPath, err := firstNonDirPath(destDir, parent)
	if err != nil {
		return err
	}
	if conflictPath != "" {
		return ArchiveEntryConflictError{
			ArchivePath:  archivePath,
			EntryName:    entryName,
			ConflictPath: conflictPath,
			Reason:       "parent path resolves through a file",
		}
	}

	fi, err := os.Stat(targetPath)
	if err == nil {
		if fi.IsDir() {
			return ArchiveEntryConflictError{
				ArchivePath:  archivePath,
				EntryName:    entryName,
				ConflictPath: targetPath,
				Reason:       "target path is already a directory",
			}
		}
		return nil
	}
	if os.IsNotExist(err) {
		return nil
	}

	return err
}

func firstNonDirPath(baseDir, targetPath string) (string, error) {
	rel, err := filepath.Rel(baseDir, targetPath)
	if err != nil {
		return "", err
	}
	if rel == "." {
		return "", nil
	}

	current := baseDir
	for _, segment := range strings.Split(rel, string(os.PathSeparator)) {
		current = filepath.Join(current, segment)

		fi, err := os.Stat(current)
		if os.IsNotExist(err) {
			return "", nil
		}
		if err != nil {
			return "", err
		}
		if !fi.IsDir() {
			return current, nil
		}
	}

	return "", nil
}
