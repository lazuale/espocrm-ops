package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestExecuteRejectsUnknownCommandAsUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := Execute([]string{"status"}, &stdout, &stderr)

	if code != exitUsage {
		t.Fatalf("unexpected exit code: %d", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout must stay empty on errors: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "unknown command") {
		t.Fatalf("unexpected stderr: %q", stderr.String())
	}
}

func TestExecuteRejectsJSONMode(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := Execute([]string{"doctor", "--json"}, &stdout, &stderr)

	if code != exitUsage {
		t.Fatalf("unexpected exit code: %d", code)
	}
	if !strings.Contains(stderr.String(), "unknown flag: --json") {
		t.Fatalf("unexpected stderr: %q", stderr.String())
	}
}
