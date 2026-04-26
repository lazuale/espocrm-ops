package main

import "time"

func backup() error {
	dir := env("BACKUP_ROOT") + "/" + time.Now().UTC().Format("20060102-150405")

	if err := sh("mkdir -p " + dir); err != nil {
		return err
	}
	if err := sh("docker compose --env-file .env -f compose.yaml exec -T -e MYSQL_PWD db mariadb-dump --single-transaction --quick --routines --triggers --events --hex-blob --default-character-set=utf8mb4 -u "+env("DB_USER")+" "+env("DB_NAME")+" | gzip -c > "+dir+"/db.sql.gz", "MYSQL_PWD="+env("DB_PASSWORD")); err != nil {
		return err
	}
	if err := sh("tar -C " + env("ESPO_STORAGE_DIR") + " -czf " + dir + "/files.tar.gz ."); err != nil {
		return err
	}
	return sh("cd " + dir + " && sha256sum db.sql.gz files.tar.gz > SHA256SUMS")
}
