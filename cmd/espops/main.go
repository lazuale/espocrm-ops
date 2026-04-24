package main

import (
	"os"

	v3cli "github.com/lazuale/espocrm-ops/internal/v3/cli"
)

func main() {
	os.Exit(v3cli.Execute(os.Args[1:], os.Stdout, os.Stderr))
}
