package backup

import (
	"archive/tar"
	"fmt"
	pathpkg "path"
	"strings"
)

func ValidateFilesArchiveHeader(hdr *tar.Header) error {
	name := pathpkg.Clean(hdr.Name)

	if name == "." || name == "/" {
		return nil
	}
	if pathpkg.IsAbs(name) {
		return fmt.Errorf("absolute path is not allowed: %s", hdr.Name)
	}
	if name == ".." || strings.HasPrefix(name, "../") {
		return fmt.Errorf("path traversal is not allowed: %s", hdr.Name)
	}

	switch hdr.Typeflag {
	case tar.TypeDir, tar.TypeReg, tar.TypeRegA:
		return nil
	case tar.TypeSymlink:
		return fmt.Errorf("symlink entries are not allowed: %s", hdr.Name)
	case tar.TypeLink:
		return fmt.Errorf("hardlink entries are not allowed: %s", hdr.Name)
	default:
		return fmt.Errorf("unsupported tar entry type %d for %s", hdr.Typeflag, hdr.Name)
	}
}
