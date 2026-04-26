package main

import (
	"os"
	"os/exec"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		os.Exit(1)
	}

	var err error
	switch os.Args[1] {
	case "backup":
		err = backup()
	case "restore":
		if len(os.Args) < 3 {
			os.Exit(1)
		}
		err = restore(os.Args[2])
	default:
		os.Exit(1)
	}

	if err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
}

func env(key string) string {
	data, err := os.ReadFile(".env")
	if err != nil {
		panic(err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, key+"=") {
			return strings.TrimPrefix(line, key+"=")
		}
	}
	return ""
}

func sh(s string, env ...string) error {
	cmd := exec.Command("bash", "-o", "pipefail", "-c", s)
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
