package main

import (
	"os"

	"github.com/lazuale/espocrm-ops/internal/architecture"
)

func main() {
	app := architecture.NewApp()
	root := app.RootCmd()
	root.SetOut(os.Stdout)
	root.SetErr(os.Stderr)
	os.Exit(app.ExecuteRoot(root))
}
