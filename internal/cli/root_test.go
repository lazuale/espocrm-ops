package cli

import "testing"

func TestRootCommandsAreMinimalV2Set(t *testing.T) {
	root := NewRootCmd()
	got := map[string]bool{}
	for _, cmd := range root.Commands() {
		got[cmd.Name()] = true
	}
	for _, name := range []string{"doctor", "backup", "restore"} {
		if !got[name] {
			t.Fatalf("root command %q is missing", name)
		}
	}
	if got["migrate"] {
		t.Fatal("migrate command must not be registered")
	}

	var hasVerify bool
	for _, cmd := range root.Commands() {
		if cmd.Name() != "backup" {
			continue
		}
		for _, sub := range cmd.Commands() {
			if sub.Name() == "verify" {
				hasVerify = true
			}
		}
	}
	if !hasVerify {
		t.Fatal("backup verify command is missing")
	}
}
