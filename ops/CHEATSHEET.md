# Шпаргалка администратора

Рабочий каталог:

```bash
cd /opt/espo
```

## Первый запуск

```bash
cp .env.prod.example .env.prod
cp .env.dev.example .env.dev
chmod +x scripts/*.sh
./scripts/doctor.sh all
./scripts/bootstrap.sh prod
./scripts/bootstrap.sh dev
./scripts/stack.sh prod up -d
./scripts/stack.sh dev up -d
```

## Основные команды

### Старт

```bash
./scripts/stack.sh prod up -d
./scripts/stack.sh dev up -d
```

### Обновление образов и запуск

```bash
./scripts/stack.sh prod pull
./scripts/stack.sh dev pull
./scripts/stack.sh prod up -d
./scripts/stack.sh dev up -d
```

Для регламентного обновления удобнее:

```bash
./scripts/update.sh prod
./scripts/update.sh dev
```

Скрипт сам ставит maintenance-lock на контур, поэтому параллельный `backup` или `restore` того же контура не запустится.

### Остановка

```bash
./scripts/stack.sh prod stop
./scripts/stack.sh dev stop
```

### Перезапуск

```bash
./scripts/stack.sh prod restart
./scripts/stack.sh dev restart
```

### Удаление контейнеров без удаления данных

```bash
./scripts/stack.sh prod down
./scripts/stack.sh dev down
```

### Статус

```bash
./scripts/stack.sh prod ps
./scripts/stack.sh dev ps
```

### Preflight-проверка

```bash
./scripts/doctor.sh all
./scripts/doctor.sh prod
./scripts/doctor.sh dev
```

Машинно-читаемый вариант:

```bash
./scripts/doctor.sh all --json
./scripts/doctor.sh prod --json
```

### Smoke-test

```bash
./scripts/smoke-test.sh dev --from-example
```

### Статус контура

```bash
./scripts/status-report.sh prod
./scripts/status-report.sh dev --json
```

JSON-отчет также показывает lock-файлы контура и последние manifests/reports/support bundle.

### Support bundle

```bash
./scripts/support-bundle.sh prod
./scripts/support-bundle.sh dev --tail 500
```

Старые bundle автоматически очищаются по `SUPPORT_RETENTION_DAYS`. Внутри архива теперь лежат и `doctor.txt`, и `doctor.json`.

### Безопасный Docker cleanup

```bash
./scripts/docker-cleanup.sh
./scripts/docker-cleanup.sh --apply
./scripts/docker-cleanup.sh --apply --include-unused-images
```

Скрипт по умолчанию только показывает план очистки. Он намеренно не делает `volume prune`, поэтому рабочие Docker-тома не затрагиваются.

### Логи

```bash
./scripts/stack.sh prod logs -f espocrm
./scripts/stack.sh dev logs -f espocrm
```

## Бэкапы

```bash
./scripts/backup.sh prod
./scripts/backup.sh dev
```

По умолчанию backup кратко останавливает прикладные сервисы ради более консистентного снимка `БД + файлы`. Если нужен быстрый, но менее надежный вариант без остановки приложения:

```bash
./scripts/backup.sh prod --no-stop
```

## Проверка бэкапов

```bash
./scripts/verify-backup.sh prod
./scripts/verify-backup.sh dev
```

## Аудит свежести бэкапов

```bash
./scripts/backup-audit.sh prod
./scripts/backup-audit.sh dev --json
```

## Каталог backup-наборов

```bash
./scripts/backup-catalog.sh prod
./scripts/backup-catalog.sh dev --ready-only --verify-checksum
./scripts/backup-catalog.sh prod --verify-checksum
```

## Drill-восстановление последних backup

```bash
./scripts/restore-drill.sh prod
./scripts/restore-drill.sh dev --timeout 900
```

Полезно прогонять после изменений в backup/restore-логике и перед серьезными работами с продом.

## Аварийный rollback

```bash
./scripts/rollback.sh prod
./scripts/rollback.sh dev --timeout 900
./scripts/rollback.sh prod --no-snapshot --no-start
```

По умолчанию rollback сам найдет последний валидный backup-набор и попытается вернуть контур в рабочее состояние.

## systemd-таймеры

Готовые шаблоны лежат в `ops/systemd/`.

После копирования в `/etc/systemd/system/`:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now espo-backup-prod.timer
sudo systemctl enable --now espo-backup-dev.timer
sudo systemctl enable --now espo-backup-audit-prod.timer
sudo systemctl enable --now espo-backup-audit-dev.timer
```

## Восстановление БД

```bash
./scripts/restore-db.sh prod /opt/espo/backups/prod/db/espocrm-prod_YYYY-MM-DD_HH-MM-SS.sql.gz
./scripts/restore-db.sh dev /opt/espo/backups/dev/db/espocrm-dev_YYYY-MM-DD_HH-MM-SS.sql.gz
```

Полезно: по умолчанию скрипт сам остановит прикладные сервисы и поднимет их обратно после успешного restore.
Для аварийного снимка перед восстановлением используйте `--snapshot-before-restore`.

## Восстановление файлов

```bash
./scripts/restore-files.sh prod /opt/espo/backups/prod/files/espocrm-prod_files_YYYY-MM-DD_HH-MM-SS.tar.gz
./scripts/restore-files.sh dev /opt/espo/backups/dev/files/espocrm-dev_files_YYYY-MM-DD_HH-MM-SS.tar.gz
```

Полезно: по умолчанию скрипт сам остановит прикладные сервисы и поднимет их обратно после успешного restore.
Для аварийного снимка перед восстановлением используйте `--snapshot-before-restore`.

## Миграция между контурами

```bash
./scripts/migrate-backup.sh dev prod
./scripts/migrate-backup.sh prod dev
```

## Предупреждение

Не используйте `down -v`, если не уверены, что именно удаляете.

`./scripts/docker-cleanup.sh` тоже специально не трогает Docker volumes: для данных EspoCRM это дополнительная защита, а не ограничение.

Если maintenance-скрипт сообщает о lock-файле в `backups/<контур>/locks/`, сначала убедитесь, что другой `backup`, `restore`, `migrate` или `update` действительно не выполняется.

Перед релизом и перед обновлением прода см. также `ops/RELEASE.md`.
