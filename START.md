# Стартовая инструкция

Этот файл нужен для самого быстрого первого запуска без чтения полного `README.md`.

## Что должно быть установлено

- Docker Engine
- Docker Compose plugin
- Bash

## 1. Подготовить env-файлы

```bash
cp .env.prod.example .env.prod
cp .env.dev.example .env.dev
```

Обязательно отредактируйте:

- `DB_ROOT_PASSWORD`
- `DB_PASSWORD`
- `ADMIN_PASSWORD`
- `SITE_URL`
- `WS_PUBLIC_URL`
- при необходимости `BACKUP_RETENTION_DAYS`, `REPORT_RETENTION_DAYS`, `SUPPORT_RETENTION_DAYS`

## 2. Сделать скрипты исполняемыми

```bash
chmod +x scripts/*.sh
```

## 3. Проверить окружение и конфигурацию

```bash
./scripts/doctor.sh all
```

## 4. Подготовить каталоги данных

```bash
./scripts/bootstrap.sh prod
./scripts/bootstrap.sh dev
```

## 5. Запустить контуры

```bash
./scripts/stack.sh prod up -d
./scripts/stack.sh dev up -d
```

## 6. Проверить состояние

```bash
./scripts/stack.sh prod ps
./scripts/stack.sh dev ps
```

## 7. Посмотреть логи при первом старте

```bash
./scripts/stack.sh prod logs -f espocrm
./scripts/stack.sh dev logs -f espocrm
```

## Полезный минимум

Создать бэкап:

```bash
./scripts/backup.sh prod
./scripts/backup.sh dev
```

По умолчанию backup кратко останавливает прикладные сервисы, чтобы снять более консистентное состояние `БД + файлы`. Планируйте такие бэкапы на окно низкой активности.

Проверить целостность последнего бэкапа:

```bash
./scripts/verify-backup.sh prod
./scripts/verify-backup.sh dev
```

Проверить свежесть и полноту последних бэкапов:

```bash
./scripts/backup-audit.sh prod
./scripts/backup-audit.sh dev --json
```

Посмотреть, какие backup-наборы реально доступны для restore:

```bash
./scripts/backup-catalog.sh prod
./scripts/backup-catalog.sh dev --ready-only --verify-checksum
```

Проверить, что backup реально разворачивается в отдельный временный контур:

```bash
./scripts/restore-drill.sh prod
./scripts/restore-drill.sh dev
```

Если контур уже сломан и нужно срочно вернуть последнее валидное состояние:

```bash
./scripts/rollback.sh prod
./scripts/rollback.sh dev
```

Получить быстрый статус контура:

```bash
./scripts/status-report.sh prod
./scripts/status-report.sh dev --json
```

JSON-вариант отчета показывает не только сервисы и каталоги, но и lock-состояние контура с последними служебными артефактами.

Безопасно обновить контур:

```bash
./scripts/update.sh prod
./scripts/update.sh dev
```

`update.sh` сам снимет статус, прогонит `doctor`, сделает контрольный backup и не даст пересечься с другим maintenance-запуском того же контура.

Посмотреть безопасный план cleanup Docker-хоста:

```bash
./scripts/docker-cleanup.sh
```

Если план устраивает, выполнить реальную очистку:

```bash
./scripts/docker-cleanup.sh --apply
```

Собрать support bundle для разбора проблем:

```bash
./scripts/support-bundle.sh prod
./scripts/support-bundle.sh dev
```

Старые диагностические архивы будут убираться автоматически по `SUPPORT_RETENTION_DAYS`, а внутри bundle уже будут и `doctor.txt`, и `doctor.json`.

Остановить контуры:

```bash
./scripts/stack.sh prod stop
./scripts/stack.sh dev stop
```

Обновить образы и перезапустить:

```bash
./scripts/stack.sh prod pull
./scripts/stack.sh dev pull
./scripts/stack.sh prod up -d
./scripts/stack.sh dev up -d
```

Прогнать изолированный smoke-test:

```bash
./scripts/smoke-test.sh dev --from-example
```

Для автоматических backup-задач на Linux можно использовать примеры из `ops/systemd/` или `ops/cron.example`.

Если нужен полный сценарий обслуживания, восстановления и миграции бэкапов, см. `README.md` и `ops/CHEATSHEET.md`.

Историю заметных изменений см. в `CHANGELOG.md`.
