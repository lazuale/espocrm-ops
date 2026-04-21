package config

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

var errNoContourToken = errors.New("no contour token")
var errAmbiguousContourToken = errors.New("ambiguous contour token")

func resolveLoadedEnvContour(path, requestedScope, declaredContour string) (string, error) {
	requestedScope = strings.TrimSpace(requestedScope)
	declaredContour = strings.TrimSpace(declaredContour)

	pathContour, err := inferEnvFileContourFromPath(path)
	if err != nil {
		if errors.Is(err, errAmbiguousContourToken) {
			return "", InvalidEnvFileError{Path: path, Message: "env filename contains both dev and prod contour tokens"}
		}
		pathContour = ""
	}

	if declaredContour != "" && !supportedContour(declaredContour) {
		return "", InvalidEnvFileError{Path: path, Message: fmt.Sprintf("ESPO_CONTOUR in the env file must be dev or prod: %s", declaredContour)}
	}
	if pathContour != "" && declaredContour != "" && pathContour != declaredContour {
		return "", InvalidEnvFileError{Path: path, Message: fmt.Sprintf("env filename points to contour %q, but ESPO_CONTOUR=%s", pathContour, declaredContour)}
	}

	effective := declaredContour
	if effective == "" {
		effective = pathContour
	}
	if effective == "" {
		return "", InvalidEnvFileError{Path: path, Message: "could not determine env file contour; add ESPO_CONTOUR=dev|prod or use a filename containing a dev/prod token"}
	}
	if effective != requestedScope {
		return "", InvalidEnvFileError{Path: path, Message: fmt.Sprintf("env file %q belongs to contour %q, but the command was run for %q", path, effective, requestedScope)}
	}

	return effective, nil
}

func inferEnvFileContourFromPath(path string) (string, error) {
	return inferContourTokenFromText(filepath.Base(path))
}

func inferContourTokenFromText(value string) (string, error) {
	found := ""
	for _, contour := range []string{"dev", "prod"} {
		if containsContourToken(value, contour) {
			if found != "" && found != contour {
				return "", errAmbiguousContourToken
			}
			found = contour
		}
	}
	if found == "" {
		return "", errNoContourToken
	}

	return found, nil
}

func containsContourToken(text, token string) bool {
	offset := 0
	for {
		idx := strings.Index(text[offset:], token)
		if idx < 0 {
			return false
		}
		start := offset + idx
		beforeOK := start == 0 || !isAlphaNum(rune(text[start-1]))
		afterIdx := start + len(token)
		afterOK := afterIdx >= len(text) || !isAlphaNum(rune(text[afterIdx]))
		if beforeOK && afterOK {
			return true
		}
		offset = start + 1
	}
}

func isAlphaNum(ch rune) bool {
	return ('a' <= ch && ch <= 'z') || ('A' <= ch && ch <= 'Z') || ('0' <= ch && ch <= '9')
}

func supportedContour(value string) bool {
	switch value {
	case "dev", "prod":
		return true
	default:
		return false
	}
}
