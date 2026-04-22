package docker

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func ReconcileEspoStoragePermissions(targetDir, helperImage string, runtimeUID, runtimeGID int) error {
	targetDir = filepath.Clean(strings.TrimSpace(targetDir))
	if targetDir == "" {
		return fmt.Errorf("storage target dir is required")
	}
	if fi, err := os.Stat(targetDir); err != nil {
		return fmt.Errorf("stat storage target dir %s: %w", targetDir, err)
	} else if !fi.IsDir() {
		return fmt.Errorf("storage target dir %s must be a directory", targetDir)
	}
	if runtimeUID < 0 {
		return fmt.Errorf("ESPO_RUNTIME_UID must be non-negative")
	}
	if runtimeGID < 0 {
		return fmt.Errorf("ESPO_RUNTIME_GID must be non-negative")
	}

	helperImage, err := ensureHelperImageAvailable(helperImage)
	if err != nil {
		return err
	}

	if _, err := runDockerCommandWithOptions(commandOptions{
		Env: []string{
			fmt.Sprintf("ESPO_RUNTIME_UID=%d", runtimeUID),
			fmt.Sprintf("ESPO_RUNTIME_GID=%d", runtimeGID),
		},
	},
		"run",
		"--pull=never",
		"--rm",
		"--user", "0:0",
		"--entrypoint", "sh",
		"-v", targetDir+":/espo-storage",
		"-e", "ESPO_RUNTIME_UID",
		"-e", "ESPO_RUNTIME_GID",
		helperImage,
		"-euc",
		reconcileEspoStoragePermissionsScript,
	); err != nil {
		return fmt.Errorf("reconcile EspoCRM storage permissions under %s: %w", targetDir, err)
	}

	return nil
}

const reconcileEspoStoragePermissionsScript = `
storage="/espo-storage"

chown -R "$ESPO_RUNTIME_UID:$ESPO_RUNTIME_GID" "$storage"
find "$storage" -type d -exec chmod 0755 {} +

for relative in data custom client/custom upload; do
  path="$storage/$relative"
  if [ -d "$path" ]; then
    find "$path" -type d -exec chmod 0775 {} +
    find "$path" -type f -exec chmod 0664 {} +
  fi
done
`
