package fs

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"
)

const legacyTarRegularType = byte(0)

func VerifyGzipReadable(filePath string) (err error) {
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer closeArchiveResource(f, fmt.Sprintf("archive file %s", filePath), &err)

	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer closeArchiveResource(gz, fmt.Sprintf("gzip reader for %s", filePath), &err)

	_, err = io.Copy(io.Discard, gz)
	return err
}

func VerifyTarGzReadable(filePath string, validateHeader func(*tar.Header) error) (err error) {
	return walkTarGz(filePath, validateHeader, func(string, *tar.Header, *tar.Reader) error {
		return nil
	})
}

func UnpackTarGz(archivePath, destDir string, validateHeader func(*tar.Header) error) error {
	return walkTarGz(archivePath, validateHeader, func(name string, hdr *tar.Header, tr *tar.Reader) error {
		targetPath := filepath.Join(destDir, filepath.FromSlash(name))
		rel, err := filepath.Rel(destDir, targetPath)
		if err != nil {
			return err
		}
		if rel == ".." || hasFilesystemDotDotPrefix(rel) {
			return archiveEntryEscapeError{ArchivePath: archivePath, EntryName: hdr.Name}
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
			return archiveUnexpectedEntryTypeError{ArchivePath: archivePath, EntryName: hdr.Name, Typeflag: hdr.Typeflag}
		}

		return nil
	})
}

func walkTarGz(filePath string, validateHeader func(*tar.Header) error, handleEntry func(string, *tar.Header, *tar.Reader) error) (err error) {
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer closeArchiveResource(f, fmt.Sprintf("archive file %s", filePath), &err)

	gz, err := gzip.NewReader(f)
	if err != nil {
		return archiveReadError{Path: filePath, Err: err}
	}
	defer closeArchiveResource(gz, fmt.Sprintf("gzip reader for %s", filePath), &err)

	tr := tar.NewReader(gz)
	var found bool

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return archiveReadError{Path: filePath, Err: err}
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

		if handleEntry != nil {
			if err := handleEntry(cleanName, hdr, tr); err != nil {
				return err
			}
			continue
		}

		if _, err := io.Copy(io.Discard, tr); err != nil {
			return archiveReadError{Path: filePath, Err: err}
		}
	}

	if !found {
		return archiveEmptyError{ArchivePath: filePath}
	}

	return nil
}

func closeArchiveResource(closer interface{ Close() error }, label string, errp *error) {
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

func hasFilesystemDotDotPrefix(p string) bool {
	p = filepath.Clean(p)
	return p == ".." || strings.HasPrefix(p, ".."+string(os.PathSeparator))
}

func ensureArchiveDirTarget(destDir, archivePath, entryName, targetPath string) error {
	conflictPath, err := firstNonDirOrSymlinkPath(destDir, targetPath)
	if err != nil {
		return err
	}
	if conflictPath != "" {
		return archiveEntryConflictError{
			ArchivePath:  archivePath,
			EntryName:    entryName,
			ConflictPath: conflictPath,
			Reason:       "directory path resolves through a file or symlink",
		}
	}

	return nil
}

func ensureArchiveFileTarget(destDir, archivePath, entryName, targetPath string) error {
	parent := filepath.Dir(targetPath)
	conflictPath, err := firstNonDirOrSymlinkPath(destDir, parent)
	if err != nil {
		return err
	}
	if conflictPath != "" {
		return archiveEntryConflictError{
			ArchivePath:  archivePath,
			EntryName:    entryName,
			ConflictPath: conflictPath,
			Reason:       "parent path resolves through a file or symlink",
		}
	}

	fi, err := os.Lstat(targetPath)
	if err == nil {
		if fi.Mode()&os.ModeSymlink != 0 {
			return archiveEntryConflictError{
				ArchivePath:  archivePath,
				EntryName:    entryName,
				ConflictPath: targetPath,
				Reason:       "target path is already a symlink",
			}
		}
		if fi.IsDir() {
			return archiveEntryConflictError{
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

func firstNonDirOrSymlinkPath(baseDir, targetPath string) (string, error) {
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

		fi, err := os.Lstat(current)
		if os.IsNotExist(err) {
			return "", nil
		}
		if err != nil {
			return "", err
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			return current, nil
		}
		if !fi.IsDir() {
			return current, nil
		}
	}

	return "", nil
}
