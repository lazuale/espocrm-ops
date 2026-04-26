package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/lazuale/espocrm-ops/internal/config"
	"github.com/lazuale/espocrm-ops/internal/ops"
)

const (
	exitOK    = 0
	exitError = 1
	exitUsage = 2
)

var errUsage = errors.New("usage")

func Execute(args []string, stdout, stderr io.Writer) int {
	if err := execute(context.Background(), args, stdout); err != nil {
		if errors.Is(err, errUsage) {
			fmt.Fprintln(stderr, err.Error())
			return exitUsage
		}
		fmt.Fprintf(stderr, "error: %v\n", err)
		return exitError
	}
	return exitOK
}

func execute(ctx context.Context, args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return usage("expected command: doctor, backup, backup verify, restore")
	}

	switch args[0] {
	case "doctor":
		values, err := parseFlags(args[1:], map[string]string{
			"scope":       "",
			"project-dir": ".",
		})
		if err != nil {
			return err
		}
		cfg, err := config.Load(values["scope"], values["project-dir"])
		if err != nil {
			return err
		}
		if err := ops.Doctor(ctx, cfg); err != nil {
			return err
		}
		fmt.Fprintln(stdout, "doctor ok")
		return nil

	case "backup":
		if len(args) > 1 && args[1] == "verify" {
			values, err := parseFlags(args[2:], map[string]string{"manifest": ""})
			if err != nil {
				return err
			}
			manifestPath, err := manifestArg(values["manifest"])
			if err != nil {
				return err
			}
			if _, err := ops.VerifyBackup(ctx, manifestPath); err != nil {
				return err
			}
			fmt.Fprintf(stdout, "backup verified: %s\n", manifestPath)
			return nil
		}
		values, err := parseFlags(args[1:], map[string]string{
			"scope":       "",
			"project-dir": ".",
		})
		if err != nil {
			return err
		}
		cfg, err := config.Load(values["scope"], values["project-dir"])
		if err != nil {
			return err
		}
		result, err := ops.Backup(ctx, cfg, time.Now().UTC())
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "backup created: %s\n", result.Manifest)
		return nil

	case "restore":
		values, err := parseFlags(args[1:], map[string]string{
			"scope":       "",
			"project-dir": ".",
			"manifest":    "",
		})
		if err != nil {
			return err
		}
		manifestPath, err := manifestArg(values["manifest"])
		if err != nil {
			return err
		}
		cfg, err := config.Load(values["scope"], values["project-dir"])
		if err != nil {
			return err
		}
		if err := ops.Restore(ctx, cfg, manifestPath); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "restore completed: %s\n", manifestPath)
		return nil

	default:
		return usage("unknown command: " + args[0])
	}
}

func parseFlags(args []string, defaults map[string]string) (map[string]string, error) {
	values := make(map[string]string, len(defaults))
	for key, value := range defaults {
		values[key] = value
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "--") {
			return nil, usage("unexpected argument: " + arg)
		}
		key := strings.TrimPrefix(arg, "--")
		if _, ok := values[key]; !ok {
			return nil, usage("unknown flag: --" + key)
		}
		if i+1 >= len(args) {
			return nil, usage("missing value for --" + key)
		}
		i++
		values[key] = args[i]
	}
	return values, nil
}

func manifestArg(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", usage("--manifest is required")
	}
	path, err := filepath.Abs(filepath.Clean(value))
	if err != nil {
		return "", err
	}
	return path, nil
}

func usage(message string) error {
	return fmt.Errorf("%w: %s", errUsage, message)
}
