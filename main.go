package main

import (
	"fmt"
	"os"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: espops backup | check-backup <backup-dir> | restore <backup-dir> --yes")
	}

	switch args[0] {
	case "backup":
		if len(args) != 1 {
			return fmt.Errorf("usage: espops backup")
		}
		cfg, err := LoadConfig(".env")
		if err != nil {
			return err
		}
		return Backup(cfg)
	case "check-backup":
		if len(args) != 2 {
			return fmt.Errorf("usage: espops check-backup <backup-dir>")
		}
		if err := ValidateBackup(args[1]); err != nil {
			return err
		}
		fmt.Println("backup OK")
		return nil
	case "restore":
		cfg, err := LoadConfig(".env")
		if err != nil {
			return err
		}
		if len(args) != 3 || args[2] != "--yes" {
			return fmt.Errorf("usage: espops restore <backup-dir> --yes")
		}
		return Restore(cfg, args[1])
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}
