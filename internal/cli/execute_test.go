package cli

import (
	"strings"
	"testing"

	"github.com/lazuale/espocrm-ops/internal/contract/exitcode"
)

func TestExecuteRoot_JSONUsageErrorRendersStructuredOutput(t *testing.T) {
	root := newTestRootCmd()
	out := &strings.Builder{}
	errOut := &strings.Builder{}

	root.SetOut(out)
	root.SetErr(errOut)
	root.SetArgs([]string{"--json", "totally-unknown-command"})

	got := ExecuteRoot(root)
	if got != exitcode.UsageError {
		t.Fatalf("expected usage exit code, got %d", got)
	}
	if errOut.Len() != 0 {
		t.Fatalf("expected no stderr for json error, got %q", errOut.String())
	}
	if !strings.Contains(out.String(), `"error"`) {
		t.Fatalf("expected structured json error output, got %s", out.String())
	}
}

func TestExecuteRoot_TextUsageErrorWritesToStderr(t *testing.T) {
	root := newTestRootCmd()
	out := &strings.Builder{}
	errOut := &strings.Builder{}

	root.SetOut(out)
	root.SetErr(errOut)
	root.SetArgs([]string{"totally-unknown-command"})

	got := ExecuteRoot(root)
	if got != exitcode.UsageError {
		t.Fatalf("expected usage exit code, got %d", got)
	}
	if out.Len() != 0 {
		t.Fatalf("expected no stdout for text error, got %q", out.String())
	}
	if !strings.Contains(errOut.String(), `ERROR: unknown command "totally-unknown-command" for "espops"`) {
		t.Fatalf("expected stderr error output, got %q", errOut.String())
	}
}
