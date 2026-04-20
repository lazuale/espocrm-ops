package cli

import (
	"os"
	"strings"
)

func envFileContourHint() string {
	return strings.TrimSpace(os.Getenv("ESPO_ENV_FILE_CONTOUR"))
}
