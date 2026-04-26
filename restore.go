package main

func restore(dir string) error {
	db := env("DB_NAME")

	if err := sh("cd " + dir + " && sha256sum -c SHA256SUMS"); err != nil {
		return err
	}
	if err := sh("docker compose --env-file .env -f compose.yaml exec -T -e MYSQL_PWD db mariadb -u root -e 'DROP DATABASE IF EXISTS `"+db+"`; CREATE DATABASE `"+db+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;'", "MYSQL_PWD="+env("DB_ROOT_PASSWORD")); err != nil {
		return err
	}
	if err := sh("gzip -dc "+dir+"/db.sql.gz | docker compose --env-file .env -f compose.yaml exec -T -e MYSQL_PWD db mariadb -u "+env("DB_USER")+" "+db, "MYSQL_PWD="+env("DB_PASSWORD")); err != nil {
		return err
	}
	if err := sh("rm -rf " + env("ESPO_STORAGE_DIR")); err != nil {
		return err
	}
	if err := sh("mkdir -p " + env("ESPO_STORAGE_DIR")); err != nil {
		return err
	}
	return sh("tar -xzf " + dir + "/files.tar.gz -C " + env("ESPO_STORAGE_DIR"))
}
