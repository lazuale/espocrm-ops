package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
)

type checksumFile struct {
	Hash string
	Size int64
}

func fileSHA256(path string) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()

	hash := sha256.New()
	size, err := io.Copy(hash, file)
	if err != nil {
		return "", 0, fmt.Errorf("hash %s: %w", path, err)
	}
	return hex.EncodeToString(hash.Sum(nil)), size, nil
}

func parseSHA256SUMS(data []byte) (map[string]string, error) {
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil, fmt.Errorf("SHA256SUMS is empty")
	}

	out := map[string]string{}
	for i, line := range lines {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			return nil, fmt.Errorf("SHA256SUMS line %d: expected '<sha256> <file>'", i+1)
		}
		hash, name := fields[0], strings.TrimPrefix(fields[1], "*")
		if len(hash) != sha256.Size*2 {
			return nil, fmt.Errorf("SHA256SUMS line %d: invalid hash length", i+1)
		}
		if _, err := hex.DecodeString(hash); err != nil {
			return nil, fmt.Errorf("SHA256SUMS line %d: invalid hash", i+1)
		}
		if name != dbFileName && name != filesFileName {
			return nil, fmt.Errorf("SHA256SUMS line %d: unexpected file %s", i+1, name)
		}
		if _, exists := out[name]; exists {
			return nil, fmt.Errorf("SHA256SUMS line %d: duplicate file %s", i+1, name)
		}
		out[name] = strings.ToLower(hash)
	}

	for _, name := range []string{dbFileName, filesFileName} {
		if _, ok := out[name]; !ok {
			return nil, fmt.Errorf("SHA256SUMS missing %s", name)
		}
	}
	return out, nil
}

func writeSHA256SUMS(path string, dbHash string, filesHash string) error {
	data := fmt.Sprintf("%s  %s\n%s  %s\n", dbHash, dbFileName, filesHash, filesFileName)
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		return fmt.Errorf("write SHA256SUMS: %w", err)
	}
	return nil
}
