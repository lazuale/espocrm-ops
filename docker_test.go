package main

import (
	"errors"
	"strings"
	"testing"
)

func TestDockerCommandErrorRedactsMYSQLPWD(t *testing.T) {
	err := dockerCommandError("test", []string{"compose", "exec", "-e", "MYSQL_PWD=supersecret", "db"}, errors.New("failed"))
	if strings.Contains(err.Error(), "supersecret") {
		t.Fatalf("password leaked in error: %v", err)
	}
	if !strings.Contains(err.Error(), "MYSQL_PWD=<redacted>") {
		t.Fatalf("redacted password marker missing: %v", err)
	}
}
