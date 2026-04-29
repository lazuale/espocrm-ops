package main

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

func createFilesArchive(sourceDir string, outPath string) error {
	out, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("create %s: %w", outPath, err)
	}

	gz := gzip.NewWriter(out)
	tw := tar.NewWriter(gz)

	walkDirErr := filepath.WalkDir(sourceDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return fmt.Errorf("make archive path for %s: %w", path, err)
		}
		if rel == "." {
			return nil
		}
		name, err := safeArchiveName(rel)
		if err != nil {
			return err
		}

		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("stat %s: %w", path, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing to archive symlink %s", path)
		}
		if info.Mode()&os.ModeType != 0 && !info.IsDir() {
			return fmt.Errorf("refusing to archive unsupported file type %s", path)
		}
		if !info.IsDir() && hardlinkCount(info) > 1 {
			return fmt.Errorf("refusing to archive hardlinked file %s", path)
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return fmt.Errorf("create archive header for %s: %w", path, err)
		}
		header.Name = name
		if info.IsDir() {
			header.Name += "/"
		}
		if err := tw.WriteHeader(header); err != nil {
			return fmt.Errorf("write archive header for %s: %w", path, err)
		}
		if info.IsDir() {
			return nil
		}

		file, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("open %s: %w", path, err)
		}
		_, copyErr := io.Copy(tw, file)
		closeErr := file.Close()
		if copyErr != nil {
			if closeErr != nil {
				return fmt.Errorf("write archive content for %s: %w; close %s: %w", path, copyErr, path, closeErr)
			}
			return fmt.Errorf("write archive content for %s: %w", path, copyErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close %s: %w", path, closeErr)
		}
		return nil
	})

	var closeErrs []error
	if err := tw.Close(); err != nil {
		closeErrs = append(closeErrs, fmt.Errorf("close tar"+" writer: %w", err))
	}
	if err := gz.Close(); err != nil {
		closeErrs = append(closeErrs, fmt.Errorf("close gzip"+" writer: %w", err))
	}
	if err := out.Close(); err != nil {
		closeErrs = append(closeErrs, fmt.Errorf("close files archive: %w", err))
	}
	if walkDirErr != nil {
		return walkDirErr
	}
	return errors.Join(closeErrs...)
}

func validateFilesArchive(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open files archive: %w", err)
	}
	defer file.Close()

	gz, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("open files archive gzip: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read files archive: %w", err)
		}
		if err := validateTarHeader(header); err != nil {
			return err
		}
		if header.Typeflag == tar.TypeReg || header.Typeflag == tar.TypeRegA {
			if _, err := io.Copy(io.Discard, tr); err != nil {
				return fmt.Errorf("read archive file %s: %w", header.Name, err)
			}
		}
	}
}

func extractFilesArchive(path string, targetDir string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open files archive: %w", err)
	}
	defer file.Close()

	gz, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("open files archive gzip: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read files archive: %w", err)
		}
		if err := validateTarHeader(header); err != nil {
			return err
		}
		dest, err := safeExtractPath(targetDir, header.Name)
		if err != nil {
			return err
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(dest, fileMode(header.FileInfo().Mode(), 0755)); err != nil {
				return fmt.Errorf("create directory %s: %w", dest, err)
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
				return fmt.Errorf("create parent directory for %s: %w", dest, err)
			}
			out, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, fileMode(header.FileInfo().Mode(), 0644))
			if err != nil {
				return fmt.Errorf("create extracted file %s: %w", dest, err)
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return fmt.Errorf("write extracted file %s: %w", dest, err)
			}
			if err := out.Close(); err != nil {
				return fmt.Errorf("close extracted file %s: %w", dest, err)
			}
		default:
			return fmt.Errorf("unsupported archive entry type %d for %s", header.Typeflag, header.Name)
		}
	}
}

func validateGzipReadable(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open db dump gzip: %w", err)
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("open db dump gzip: %w", err)
	}
	defer gz.Close()
	if _, err := io.Copy(io.Discard, gz); err != nil {
		return fmt.Errorf("read db dump gzip: %w", err)
	}
	return nil
}

func validateTarHeader(header *tar.Header) error {
	if header == nil {
		return fmt.Errorf("nil archive header")
	}
	if _, err := safeArchiveName(header.Name); err != nil {
		return err
	}
	switch header.Typeflag {
	case tar.TypeDir, tar.TypeReg, tar.TypeRegA:
		return nil
	case tar.TypeSymlink, tar.TypeLink:
		return fmt.Errorf("archive entry %s uses unsafe link type", header.Name)
	default:
		return fmt.Errorf("archive entry %s uses unsupported type %d", header.Name, header.Typeflag)
	}
}

func safeArchiveName(name string) (string, error) {
	name = filepath.ToSlash(name)
	if name == "" {
		return "", fmt.Errorf("archive entry has empty path")
	}
	if strings.HasPrefix(name, "/") || filepath.IsAbs(name) {
		return "", fmt.Errorf("archive entry %s is absolute", name)
	}
	clean := filepath.ToSlash(filepath.Clean(name))
	if clean == "." {
		return "", fmt.Errorf("archive entry . is not allowed")
	}
	if clean == ".." || strings.HasPrefix(clean, "../") || strings.Contains(clean, "/../") {
		return "", fmt.Errorf("archive entry %s escapes archive root", name)
	}
	return clean, nil
}

func safeExtractPath(targetDir string, name string) (string, error) {
	clean, err := safeArchiveName(name)
	if err != nil {
		return "", err
	}
	dest := filepath.Join(targetDir, filepath.FromSlash(clean))
	inside, err := pathInside(targetDir, dest)
	if err != nil {
		return "", fmt.Errorf("validate extract path: %w", err)
	}
	if !inside {
		return "", fmt.Errorf("archive entry %s escapes extraction directory", name)
	}
	return dest, nil
}

func hardlinkCount(info os.FileInfo) uint64 {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 1
	}
	return uint64(stat.Nlink)
}

func fileMode(mode fs.FileMode, fallback fs.FileMode) fs.FileMode {
	perm := mode.Perm()
	if perm == 0 {
		return fallback
	}
	return perm
}
