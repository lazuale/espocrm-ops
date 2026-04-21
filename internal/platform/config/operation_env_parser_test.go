package config

import (
	"errors"
	"strings"
	"testing"
)

func TestParseEnvValueAllowsLiteralDollarAcrossValueForms(t *testing.T) {
	tests := []struct {
		name     string
		rawValue string
		want     string
	}{
		{name: "unquoted", rawValue: "price$usd", want: "price$usd"},
		{name: "double quoted", rawValue: "\"price$usd\"", want: "price$usd"},
		{name: "single quoted", rawValue: "'price$usd'", want: "price$usd"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseEnvValue(tt.rawValue, "test.env", 1)
			if err != nil {
				t.Fatalf("parseEnvValue failed: %v", err)
			}
			if got != tt.want {
				t.Fatalf("unexpected value: got %q want %q", got, tt.want)
			}
		})
	}
}

func TestParseEnvValueRejectsShellExpansionSyntax(t *testing.T) {
	tests := []string{
		"$(date)",
		"\"${HOME}\"",
		"'`uname`'",
	}

	for _, rawValue := range tests {
		t.Run(rawValue, func(t *testing.T) {
			_, err := parseEnvValue(rawValue, "test.env", 3)
			if err == nil {
				t.Fatal("expected parse error")
			}

			var parseErr EnvParseError
			if !errors.As(err, &parseErr) {
				t.Fatalf("expected EnvParseError, got %T: %v", err, err)
			}
			if !strings.Contains(err.Error(), "shell expansion syntax") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestParseEnvValueRejectsShellStyleEscapesInDoubleQuotes(t *testing.T) {
	_, err := parseEnvValue("\"\\$HOME\"", "test.env", 4)
	if err == nil {
		t.Fatal("expected parse error")
	}

	var parseErr EnvParseError
	if !errors.As(err, &parseErr) {
		t.Fatalf("expected EnvParseError, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "unsupported escape sequence") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadEnvAssignmentsRejectsShellLikeLines(t *testing.T) {
	path := writeOperationEnvLinesFile(t, t.TempDir(), "ops.env", []string{
		"export COMPOSE_PROJECT_NAME=espocrm-dev",
	}, 0o640)

	_, err := loadEnvAssignments(path)
	if err == nil {
		t.Fatal("expected parse error")
	}

	var parseErr EnvParseError
	if !errors.As(err, &parseErr) {
		t.Fatalf("expected EnvParseError, got %T: %v", err, err)
	}
	if parseErr.Line != 1 {
		t.Fatalf("unexpected parse line: %d", parseErr.Line)
	}
}
