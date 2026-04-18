package fs

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

func SHA256File(path string) (checksum string, err error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open file: %w", err)
	}
	defer closeArchiveResource(f, fmt.Sprintf("checksum input file %s", path), &err)

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("hash file: %w", err)
	}

	checksum = hex.EncodeToString(h.Sum(nil))
	return checksum, nil
}
