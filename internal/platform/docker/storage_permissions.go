package docker

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func ResolveEspoRuntimeOwner(espocrmImage string) (int, int, error) {
	image := strings.TrimSpace(espocrmImage)
	if image == "" {
		return 0, 0, fmt.Errorf("ESPOCRM_IMAGE is required to resolve runtime storage ownership")
	}
	if _, err := runCommand(commandOptions{Env: dockerCommandEnv()}, "docker", "image", "inspect", image); err != nil {
		return 0, 0, fmt.Errorf("inspect image %s: %w", image, err)
	}

	res, err := runCommand(commandOptions{Env: dockerCommandEnv()}, "docker",
		"run",
		"--pull=never",
		"--rm",
		"--user", "0:0",
		"--entrypoint", "sh",
		image,
		"-euc",
		`
for path in \
  /var/www/html/data \
  /var/www/html/custom \
  /var/www/html/client/custom \
  /var/www/html/upload \
  /var/www/html
do
  if [ -e "$path" ]; then
    set -- $(ls -nd "$path")
    [ "$#" -ge 4 ] || exit 1
    printf "%s:%s\n" "$3" "$4"
    exit 0
  fi
done

exit 1
`,
	)
	if err != nil {
		return 0, 0, fmt.Errorf("inspect runtime owner from image %s: %w", image, err)
	}

	owner := strings.TrimSpace(res.Stdout)
	uidRaw, gidRaw, ok := strings.Cut(owner, ":")
	if !ok {
		return 0, 0, fmt.Errorf("runtime owner %q did not match uid:gid format", owner)
	}

	uid, err := strconv.Atoi(strings.TrimSpace(uidRaw))
	if err != nil {
		return 0, 0, fmt.Errorf("parse runtime uid %q: %w", uidRaw, err)
	}
	gid, err := strconv.Atoi(strings.TrimSpace(gidRaw))
	if err != nil {
		return 0, 0, fmt.Errorf("parse runtime gid %q: %w", gidRaw, err)
	}

	return uid, gid, nil
}

func ReconcileEspoStoragePermissions(targetDir, mariaDBTag, espocrmImage string) error {
	targetDir = filepath.Clean(strings.TrimSpace(targetDir))
	if targetDir == "" {
		return fmt.Errorf("storage target dir is required")
	}
	if fi, err := os.Stat(targetDir); err != nil {
		return fmt.Errorf("stat storage target dir %s: %w", targetDir, err)
	} else if !fi.IsDir() {
		return fmt.Errorf("storage target dir %s must be a directory", targetDir)
	}

	runtimeUID, runtimeGID, err := ResolveEspoRuntimeOwner(espocrmImage)
	if err != nil {
		return err
	}

	helperImage, err := selectCleanupHelperImage(mariaDBTag, espocrmImage)
	if err != nil {
		return err
	}

	if _, err := runCommand(commandOptions{
		Env: dockerCommandEnv(
			fmt.Sprintf("ESPO_RUNTIME_UID=%d", runtimeUID),
			fmt.Sprintf("ESPO_RUNTIME_GID=%d", runtimeGID),
		),
	}, "docker",
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
		`
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
`,
	); err != nil {
		return fmt.Errorf("reconcile EspoCRM storage permissions under %s: %w", targetDir, err)
	}

	return nil
}
