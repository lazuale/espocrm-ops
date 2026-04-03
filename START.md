# Быстрый старт за 5 минут

Этот файл нужен для самого короткого первого запуска без чтения полного `README.md`.

## Что должно быть установлено

- Docker Engine
- Docker Compose plugin
- Bash

## Вариант по умолчанию

Если нужен только рабочий контур, сначала поднимайте `prod`.  
`dev` можно включить позже, когда понадобится тестовая среда.

По умолчанию:

- `prod`: `http://YOUR_SERVER_IP:8080`
- `dev`: `http://127.0.0.1:8088`
- часовой пояс: `Europe/Moscow`

## 1. Подготовить env-файл для `prod`

```bash
cp .env.prod.example .env.prod
```

Обязательно отредактируйте в `.env.prod`:

- `DB_ROOT_PASSWORD`
- `DB_PASSWORD`
- `ADMIN_PASSWORD`
- `SITE_URL`
- `WS_PUBLIC_URL`

Минимально рабочий вариант:

- `SITE_URL=http://YOUR_SERVER_IP:8080`
- `WS_PUBLIC_URL=ws://YOUR_SERVER_IP:8081`

## 2. Сделать скрипты исполняемыми

```bash
chmod +x scripts/*.sh
```

## 3. Проверить окружение

```bash
./scripts/doctor.sh prod
```

## 4. Подготовить каталоги данных

```bash
./scripts/bootstrap.sh prod
```

## 5. Запустить `prod`

```bash
./scripts/stack.sh prod up -d
```

## 6. Проверить, что все поднялось

```bash
./scripts/stack.sh prod ps
./scripts/stack.sh prod logs -f espocrm
```

На первом старте EspoCRM может подниматься несколько минут.  
После этого откройте в браузере `http://YOUR_SERVER_IP:8080`.

## 7. Если нужен еще и `dev`

```bash
cp .env.dev.example .env.dev
./scripts/doctor.sh dev
./scripts/bootstrap.sh dev
./scripts/stack.sh dev up -d
./scripts/stack.sh dev ps
```

`dev` по умолчанию работает на:

- `http://127.0.0.1:8088`
- `ws://127.0.0.1:8089`

## Полезный минимум после запуска

Создать бэкап:

```bash
./scripts/backup.sh prod
```

По умолчанию backup кратко останавливает прикладные сервисы, чтобы снять более консистентное состояние `БД + файлы`. Планируйте такие бэкапы на окно низкой активности.

Проверить целостность последнего бэкапа:

```bash
./scripts/verify-backup.sh prod
```

Проверить свежесть и полноту последних бэкапов:

```bash
./scripts/backup-audit.sh prod
```

Посмотреть, какие backup-наборы реально доступны для restore:

```bash
./scripts/backup-catalog.sh prod
```

Проверить, что backup реально разворачивается в отдельный временный контур:

```bash
./scripts/restore-drill.sh prod
```

Если контур уже сломан и нужно срочно вернуть последнее валидное состояние:

```bash
./scripts/rollback.sh prod
```

Получить быстрый статус контура:

```bash
./scripts/status-report.sh prod
```

JSON-вариант отчета показывает не только сервисы и каталоги, но и lock-состояние контура с последними служебными артефактами.

Безопасно обновить контур:

```bash
./scripts/update.sh prod
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
```

Старые диагностические архивы будут убираться автоматически по `SUPPORT_RETENTION_DAYS`, а внутри bundle уже будут и `doctor.txt`, и `doctor.json`.

Остановить контуры:

```bash
./scripts/stack.sh prod stop
```

Обновить образы и перезапустить:

```bash
./scripts/stack.sh prod pull
./scripts/stack.sh prod up -d
```

Прогнать изолированный smoke-test:

```bash
./scripts/smoke-test.sh dev --from-example
```

Для автоматических backup-задач на Linux можно использовать примеры из `ops/systemd/` или `ops/cron.example`.

Если нужен полный сценарий обслуживания, восстановления, `dev`-контура и миграции бэкапов, см. `README.md` и `ops/CHEATSHEET.md`.

Историю заметных изменений см. в `CHANGELOG.md`.
