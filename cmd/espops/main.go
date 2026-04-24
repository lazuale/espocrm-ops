package main

import (
	"os"

	cli "github.com/lazuale/espocrm-ops/internal/cli"
)

func main() {
	os.Exit(cli.Execute(os.Args[1:], os.Stdout, os.Stderr))
}
