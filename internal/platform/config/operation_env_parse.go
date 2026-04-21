package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"
)

func loadEnvAssignments(path string) (values map[string]string, err error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open env file %s: %w", path, err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			wrapped := fmt.Errorf("close env file %s: %w", path, closeErr)
			if err == nil {
				err = wrapped
			} else {
				err = errors.Join(err, wrapped)
			}
		}
	}()

	values = map[string]string{}
	scanner := bufio.NewScanner(file)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}

		key, rawValue, ok := parseEnvAssignment(line)
		if !ok {
			return nil, EnvParseError{Path: path, Line: lineNo, Message: "expected a KEY=VALUE line without shell code"}
		}
		if _, exists := values[key]; exists {
			return nil, EnvParseError{Path: path, Line: lineNo, Message: fmt.Sprintf("duplicate assignment for %s", key)}
		}

		value, err := parseEnvValue(rawValue, path, lineNo)
		if err != nil {
			return nil, err
		}
		values[key] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read env file %s: %w", path, err)
	}

	return values, nil
}

func parseEnvAssignment(line string) (string, string, bool) {
	trimmed := strings.TrimLeft(line, " \t")
	if trimmed == "" {
		return "", "", false
	}

	sep := strings.IndexByte(trimmed, '=')
	if sep <= 0 {
		return "", "", false
	}

	key := trimmed[:sep]
	for i, ch := range key {
		if i == 0 {
			if !isEnvAssignmentKeyStart(ch) {
				return "", "", false
			}
			continue
		}
		if !isEnvAssignmentKeyPart(ch) {
			return "", "", false
		}
	}

	return key, trimmed[sep+1:], true
}

func isEnvAssignmentKeyStart(ch rune) bool {
	return ch == '_' || ('A' <= ch && ch <= 'Z') || ('a' <= ch && ch <= 'z')
}

func isEnvAssignmentKeyPart(ch rune) bool {
	return isEnvAssignmentKeyStart(ch) || ('0' <= ch && ch <= '9')
}

func parseEnvValue(rawValue, path string, lineNo int) (string, error) {
	switch {
	case strings.HasPrefix(rawValue, "\""):
		return decodeDoubleQuotedEnvValue(rawValue, path, lineNo)
	case strings.HasPrefix(rawValue, "'"):
		return decodeSingleQuotedEnvValue(rawValue, path, lineNo)
	default:
		return decodeUnquotedEnvValue(rawValue, path, lineNo)
	}
}

func decodeDoubleQuotedEnvValue(rawValue, path string, lineNo int) (string, error) {
	if len(rawValue) < 2 || !strings.HasSuffix(rawValue, "\"") {
		return "", EnvParseError{Path: path, Line: lineNo, Message: "double-quoted value must end with a closing quote"}
	}

	inner := rawValue[1 : len(rawValue)-1]
	var decoded strings.Builder
	escapeNext := false

	for _, ch := range inner {
		if escapeNext {
			switch ch {
			case '\\', '"':
				decoded.WriteRune(ch)
			default:
				return "", EnvParseError{Path: path, Line: lineNo, Message: fmt.Sprintf("unsupported escape sequence \\%c", ch)}
			}
			escapeNext = false
			continue
		}

		switch ch {
		case '\\':
			escapeNext = true
		case '"':
			return "", EnvParseError{Path: path, Line: lineNo, Message: "inner double quotes must be escaped"}
		default:
			decoded.WriteRune(ch)
		}
	}

	if escapeNext {
		return "", EnvParseError{Path: path, Line: lineNo, Message: "double-quoted value ends with an unfinished escape sequence"}
	}
	if err := rejectShellSyntax(decoded.String(), path, lineNo, "double-quoted value"); err != nil {
		return "", err
	}

	return decoded.String(), nil
}

func decodeSingleQuotedEnvValue(rawValue, path string, lineNo int) (string, error) {
	if len(rawValue) < 2 || !strings.HasSuffix(rawValue, "'") {
		return "", EnvParseError{Path: path, Line: lineNo, Message: "single-quoted value must end with a closing quote"}
	}

	inner := rawValue[1 : len(rawValue)-1]
	if strings.Contains(inner, "'") {
		return "", EnvParseError{Path: path, Line: lineNo, Message: "single-quoted value must not contain a raw single quote"}
	}
	if err := rejectShellSyntax(inner, path, lineNo, "single-quoted value"); err != nil {
		return "", err
	}

	return inner, nil
}

func decodeUnquotedEnvValue(rawValue, path string, lineNo int) (string, error) {
	if strings.ContainsAny(rawValue, " \t") {
		return "", EnvParseError{Path: path, Line: lineNo, Message: "a value containing spaces must be quoted"}
	}
	if err := rejectShellSyntax(rawValue, path, lineNo, "value"); err != nil {
		return "", err
	}

	return rawValue, nil
}

func rejectShellSyntax(value, path string, lineNo int, label string) error {
	if strings.Contains(value, "$(") || strings.Contains(value, "${") || strings.Contains(value, "`") {
		return EnvParseError{Path: path, Line: lineNo, Message: label + " must not contain shell expansion syntax"}
	}

	return nil
}
