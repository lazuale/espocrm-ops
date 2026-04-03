# Инфраструктура запуска и обслуживания EspoCRM

> Готовая Docker-инфраструктура для внутреннего развертывания EspoCRM: `prod` и `dev`, резервные копии, восстановление, rollback, smoke-test и регламентное обслуживание.

Этот репозиторий нужен для быстрого и предсказуемого запуска EspoCRM на одном сервере без собственной сборки образов и без хранения приватных настроек в git.

## Коротко о проекте

- Для кого: для администратора или DevOps-инженера, которому нужно поднять и сопровождать EspoCRM по внутреннему IP внутри корпоративной сети.
- Что внутри: Docker Compose-стек, env-шаблоны, backup/restore, rollback, миграция бэкапов между `dev` и `prod`, preflight-проверки, smoke-test и диагностические отчеты.
- Чего здесь нет: кастомных модулей EspoCRM, собственной сборки Docker-образа, внешнего reverse proxy и публичного HTTPS-контура.
- Что можно запустить сразу: `prod` на `http://YOUR_SERVER_IP:8080` и `dev` на `http://127.0.0.1:8088`.
- С чего начать: откройте `START.md`, там есть короткий сценарий первого запуска на 5 минут.

Этот репозиторий содержит инфраструктурный набор для запуска и обслуживания EspoCRM в Docker с двумя независимыми контурами:

- `prod` для рабочей среды;
- `dev` для тестов, отладки и обкатки изменений.

Внутри уже настроены:

- веб-контейнер EspoCRM на Apache;
- MariaDB;
- daemon-контейнер фоновых задач;
- websocket-контейнер;
- раздельные каталоги хранения данных;
- резервное копирование и восстановление;
- миграция бэкапа между контурами;
- часовой пояс `Europe/Moscow` по умолчанию.

Важно:

- здесь нет кастомных модулей EspoCRM;
- здесь нет собственной сборки образа;
- репозиторий отвечает только за запуск, обслуживание, бэкапы и миграции между контурами.
- для самого короткого сценария первого запуска есть отдельный файл `START.md`.
- основной сценарий использования — доступ по внутренним IP-адресам внутри корпоративной сети.

## Быстрый старт

Самый короткий путь:

```bash
cp .env.prod.example .env.prod
chmod +x scripts/*.sh
./scripts/doctor.sh prod
./scripts/bootstrap.sh prod
./scripts/stack.sh prod up -d
./scripts/stack.sh prod ps
```

После этого:

- откройте `http://YOUR_SERVER_IP:8080`;
- дождитесь завершения первого старта EspoCRM;
- при необходимости позже поднимите `dev` отдельно;
- для полного пошагового сценария см. `START.md`.

## Что дает эта структура

- `dev` и `prod` можно запускать одновременно на одном сервере.
- Данные, бэкапы и порты у контуров разделены.
- Конфигурация хранится в env-файлах и не зашивается в код.
- Приватные рабочие файлы исключены через `.gitignore`.
- Скрипты сведены к одному стилю и работают через общий слой обвязки.
- Используется официальный образ EspoCRM без локальных модульных доработок.

## Структура проекта

- `START.md` — короткая стартовая инструкция.
- `CHANGELOG.md` — журнал заметных изменений.
- `compose.yaml` — основной Docker Compose-файл.
- `.env.prod.example` — шаблон env-файла для прод-контура.
- `.env.dev.example` — шаблон env-файла для dev-контура.
- `.editorconfig` — единые правила форматирования файлов в репозитории.
- `deploy/php/espocrm.ini` — локальные настройки PHP.
- `deploy/mariadb/z-custom.cnf` — тюнинг MariaDB.
- `deploy/apache/zzz-espo-tuning.conf` — тюнинг Apache.
- `scripts/bootstrap.sh` — подготовка каталогов контуров.
- `scripts/doctor.sh` — preflight-проверка окружения и конфигурации.
- `scripts/docker-cleanup.sh` — безопасный dry-run/apply cleanup Docker-хоста без `volume prune`.
- `scripts/stack.sh` — удобная обертка над `docker compose`.
- `scripts/backup.sh` — создание резервной копии.
- `scripts/backup-audit.sh` — аудит свежести и целостности последних backup-артефактов.
- `scripts/backup-catalog.sh` — каталог backup-наборов с оценкой готовности к restore.
- `scripts/restore-drill.sh` — изолированная проверка восстановления из backup во временный контур.
- `scripts/smoke-test.sh` — изолированный smoke-test жизненного цикла стека.
- `scripts/verify-backup.sh` — проверка целостности backup-файлов.
- `scripts/status-report.sh` — краткий статус-отчет по контуру в text/JSON.
- `scripts/support-bundle.sh` — сборка диагностического support bundle.
- `scripts/restore-db.sh` — восстановление базы данных.
- `scripts/restore-files.sh` — восстановление файлов.
- `scripts/rollback.sh` — аварийный rollback на последний валидный backup-набор.
- `scripts/migrate-backup.sh` — перенос бэкапа между `dev` и `prod`.
- `scripts/update.sh` — безопасное регламентное обновление контура.
- `ops/CHEATSHEET.md` — краткая шпаргалка администратора.
- `ops/cron.example` — примеры cron-заданий для резервного копирования.
- `ops/systemd/` — готовые unit-файлы и инструкция для запуска backup через `systemd timer`.
- `ops/RELEASE.md` — краткая памятка перед релизом и обновлением прода.
- `.github/workflows/ci.yml` — CI-проверки для shell-скриптов и compose-конфигурации.
- `LICENSE` — лицензия MIT.

## Первичная настройка

1. Скопируйте проект на сервер, например в `/opt/espo`.
2. Создайте рабочие env-файлы:

```bash
cp .env.prod.example .env.prod
cp .env.dev.example .env.dev
```

3. Отредактируйте `.env.prod` и `.env.dev`:

- задайте реальные пароли;
- укажите нужные `SITE_URL` и `WS_PUBLIC_URL`;
- при необходимости измените порты;
- при необходимости скорректируйте сроки хранения `BACKUP_RETENTION_DAYS`, `REPORT_RETENTION_DAYS` и `SUPPORT_RETENTION_DAYS`;
- при необходимости скорректируйте SLA-пороги `BACKUP_MAX_DB_AGE_HOURS` и `BACKUP_MAX_FILES_AGE_HOURS`;
- при необходимости скорректируйте лимиты памяти/CPU и ротацию логов контейнеров;
- проверьте пути хранения данных и бэкапов.

4. Сделайте скрипты исполняемыми:

```bash
chmod +x scripts/*.sh
```

5. Прогоните preflight-проверку:

```bash
./scripts/doctor.sh all
```

`doctor.sh` дополнительно проверяет:

- рекомендуемые версии Docker Engine и Docker Compose;
- минимальный свободный объем на файловых системах для данных и бэкапов;
- корректность числовых параметров хранения для backup/report/support артефактов;
- формат лимитов ресурсов и ротации логов.

Для автоматизации можно использовать JSON-режим:

```bash
./scripts/doctor.sh all --json
./scripts/doctor.sh prod --json
```

6. Подготовьте каталоги контуров:

```bash
./scripts/bootstrap.sh prod
./scripts/bootstrap.sh dev
```

7. Запустите нужный контур:

```bash
./scripts/stack.sh prod up -d
./scripts/stack.sh dev up -d
```

## Повседневные команды

### Проверить статус

```bash
./scripts/stack.sh prod ps
./scripts/stack.sh dev ps
```

### Прогнать preflight-проверку

```bash
./scripts/doctor.sh all
./scripts/doctor.sh prod
./scripts/doctor.sh dev
```

### Прогнать smoke-test

```bash
./scripts/smoke-test.sh dev --from-example
```

Это особенно полезно перед публикацией изменений в репозиторий или перед обновлением CI.

### Прогнать restore-drill по последним backup

```bash
./scripts/restore-drill.sh prod
./scripts/restore-drill.sh dev
```

Скрипт поднимает отдельный временный стек, восстанавливает в него последние backup БД и файлов, ждет готовности сервисов и сохраняет drill-отчет в `backups/<контур>/reports/`.
Если восстановление ломается, рядом создается support bundle для разбора причины.

### Снять статус-отчет

```bash
./scripts/status-report.sh prod
./scripts/status-report.sh dev --json
```

Отчет удобно фиксировать до изменений и после них. При необходимости его можно сразу сохранить в файл через `--output`.
JSON-отчет теперь также показывает lock-состояние контура и последние служебные артефакты: manifests, reports и support bundle.

### Собрать support bundle

```bash
./scripts/support-bundle.sh prod
./scripts/support-bundle.sh dev --tail 500
```

В bundle попадают redacted env, `docker compose config`, `docker compose ps`, tail логов, текстовый и JSON-вывод `doctor.sh`, текстовый и JSON-вывод статус-отчета и последние manifest-файлы.

Дополнительно в bundle теперь попадает текстовый и JSON-каталог backup-наборов, чтобы сразу видеть, какой backup был доступен на момент инцидента.

Старые bundle автоматически очищаются по сроку `SUPPORT_RETENTION_DAYS`.

### Посмотреть логи

```bash
./scripts/stack.sh prod logs -f espocrm
./scripts/stack.sh dev logs -f espocrm
```

### Остановить контур

```bash
./scripts/stack.sh prod stop
./scripts/stack.sh dev stop
```

### Обновить образ и перезапустить

```bash
./scripts/stack.sh prod pull
./scripts/stack.sh dev pull
./scripts/stack.sh prod up -d
./scripts/stack.sh dev up -d
```

Для более безопасного сценария обслуживания удобнее использовать:

```bash
./scripts/update.sh prod
./scripts/update.sh dev
```

`update.sh` автоматически:

- снимает pre-update статус-отчет;
- прогоняет `doctor.sh`, если не задан `--skip-doctor`;
- создает контрольный backup текущего состояния, если не задан `--skip-backup`;
- выполняет `docker compose pull`, если не задан `--skip-pull`;
- перезапускает стек и ждет готовности всех сервисов;
- проверяет HTTP-доступность приложения, если не задан `--skip-http-probe`;
- сохраняет post-update статус-отчет;
- при сбое собирает support bundle в `backups/<контур>/support/`.

Во время выполнения `update.sh` захватывает maintenance-lock, поэтому параллельный `backup` или `restore` для того же контура не стартует и завершится понятной ошибкой.

### Безопасно почистить старые Docker-артефакты

```bash
./scripts/docker-cleanup.sh
./scripts/docker-cleanup.sh --apply
./scripts/docker-cleanup.sh --apply --include-unused-images
```

Скрипт:

- по умолчанию работает только как dry-run и ничего не удаляет;
- показывает кандидатов среди старых остановленных контейнеров, dangling-образов и неиспользуемых пользовательских сетей;
- по флагу `--include-unused-images` дополнительно удаляет старые tag-образы, которые больше не используются ни одним контейнером;
- опционально очищает build cache через `docker builder prune`;
- намеренно не делает `volume prune`, чтобы не задеть Docker-тома с данными;
- перед реальным удалением проверяет maintenance-lock и ставит барьерные lock-файлы, чтобы cleanup не пересекся с `backup`, `restore`, `rollback` или `update`.

Полезные флаги:

- `--apply` — реально удалить найденные кандидаты;
- `--container-age`, `--image-age`, `--unused-image-age`, `--network-age`, `--builder-age` — изменить пороги старения;
- `--skip-build-cache` — не трогать build cache.

### Удалить контейнеры, не трогая данные

```bash
./scripts/stack.sh prod down
./scripts/stack.sh dev down
```

## Резервное копирование

### Создать бэкап

```bash
./scripts/backup.sh prod
./scripts/backup.sh dev
```

Скрипт создает:

- дамп БД в формате `.sql.gz`;
- архив файлов приложения в формате `.tar.gz`;
- checksum-файлы `.sha256` рядом с каждым backup-артефактом;
- manifest-файлы в текстовом и JSON-виде в каталоге `backups/<контур>/manifests/`;
- ротацию старых бэкапов по сроку хранения.

По умолчанию `backup.sh` кратко останавливает `espocrm`, `espocrm-daemon` и `espocrm-websocket`, затем снимает дамп БД и архив файлов, после чего поднимает сервисы обратно. Это дает более консистентную пару `DB + files`, но означает короткое окно обслуживания на время backup.

При необходимости можно создать частичный backup:

```bash
./scripts/backup.sh prod --skip-files
./scripts/backup.sh dev --skip-db
```

Если нужен более быстрый, но рискованный backup без остановки приложения, можно явно отключить консистентный режим:

```bash
./scripts/backup.sh prod --no-stop
./scripts/backup.sh dev --skip-db --no-stop
```

### Проверить целостность бэкапа

```bash
./scripts/verify-backup.sh prod
./scripts/verify-backup.sh dev
```

### Прогнать аудит свежести и целостности бэкапов

```bash
./scripts/backup-audit.sh prod
./scripts/backup-audit.sh dev --json
```

Скрипт проверяет, что последние backup-файлы не старше заданных порогов, что рядом есть `.sha256`, что контрольные суммы совпадают и что manifest-файлы присутствуют и выглядят корректно.

### Посмотреть каталог доступных backup-наборов

```bash
./scripts/backup-catalog.sh prod
./scripts/backup-catalog.sh dev --json
./scripts/backup-catalog.sh prod --ready-only --verify-checksum
```

Скрипт группирует backup-артефакты по timestamp, показывает полноту набора для restore и умеет дополнительно проверять checksum через `--verify-checksum`. Без checksum-проверки полный набор помечается как `ready_unverified`, а с checksum-проверкой как `ready_verified`.

### Прогнать drill-восстановление последних backup

```bash
./scripts/restore-drill.sh prod
./scripts/restore-drill.sh dev --timeout 900
```

Полезные флаги:

- `--db-backup` — явно указать backup БД вместо автоподбора последнего.
- `--files-backup` — явно указать backup файлов вместо автоподбора последнего.
- `--app-port` и `--ws-port` — переопределить порты временного drill-контура.
- `--skip-http-probe` — пропустить HTTP-проверку после поднятия drill-контура.
- `--keep-artifacts` — не удалять временный drill-контур после завершения.

### Выполнить аварийный rollback на последний валидный backup

```bash
./scripts/rollback.sh prod
./scripts/rollback.sh dev --timeout 900
```

По умолчанию rollback:

- ищет последний complete backup-набор с подтвержденными checksum;
- снимает аварийный snapshot текущего состояния перед перезаписью;
- восстанавливает БД и файлы;
- поднимает контур обратно и проверяет HTTP-доступность.

Полезные флаги:

- `--db-backup` и `--files-backup` — вручную указать конкретную пару backup-файлов;
- `--no-snapshot` — не снимать текущий snapshot перед rollback;
- `--no-start` — оставить контур остановленным после восстановления;
- `--skip-http-probe` — не делать HTTP-проверку после возврата контура;
- `--timeout` — увеличить таймаут ожидания сервисов.

## Восстановление

### Восстановить базу данных

```bash
./scripts/restore-db.sh prod /opt/espo/backups/prod/db/espocrm-prod_YYYY-MM-DD_HH-MM-SS.sql.gz
./scripts/restore-db.sh dev /opt/espo/backups/dev/db/espocrm-dev_YYYY-MM-DD_HH-MM-SS.sql.gz
```

Что делает скрипт:

- по умолчанию останавливает прикладные сервисы перед restore, если они были запущены;
- при наличии `.sha256` проверяет целостность дампа перед импортом;
- проверяет наличие файла;
- убеждается, что контейнер БД запущен;
- пересоздает целевую БД;
- импортирует дамп;
- после успешного restore поднимает обратно прикладные сервисы, если они были остановлены самим скриптом.

Полезные флаги:

- `--snapshot-before-restore` — перед restore создать аварийный полный backup текущего состояния.
- `--no-stop` — не останавливать прикладные сервисы автоматически.
- `--no-start` — не запускать обратно прикладные сервисы после успешного restore.

### Восстановить файлы

```bash
./scripts/restore-files.sh prod /opt/espo/backups/prod/files/espocrm-prod_files_YYYY-MM-DD_HH-MM-SS.tar.gz
./scripts/restore-files.sh dev /opt/espo/backups/dev/files/espocrm-dev_files_YYYY-MM-DD_HH-MM-SS.tar.gz
```

Что делает скрипт:

- по умолчанию останавливает прикладные сервисы перед restore, если они были запущены;
- при наличии `.sha256` проверяет целостность архива перед распаковкой;
- очищает целевое файловое хранилище;
- распаковывает архив в каталог соответствующего контура;
- после успешного restore поднимает обратно прикладные сервисы, если они были остановлены самим скриптом.

Полезные флаги:

- `--snapshot-before-restore` — перед restore создать аварийный полный backup текущего состояния.
- `--no-stop` — не останавливать прикладные сервисы автоматически.
- `--no-start` — не запускать обратно прикладные сервисы после успешного restore.

## Миграция между `dev` и `prod`

### Перенести последний доступный бэкап

```bash
./scripts/migrate-backup.sh dev prod
./scripts/migrate-backup.sh prod dev
```

### Перенести конкретные backup-файлы

```bash
./scripts/migrate-backup.sh dev prod \
  --db-backup /opt/espo/backups/dev/db/espocrm-dev_YYYY-MM-DD_HH-MM-SS.sql.gz \
  --files-backup /opt/espo/backups/dev/files/espocrm-dev_files_YYYY-MM-DD_HH-MM-SS.tar.gz
```

### Полезные флаги

- `--skip-db` — не переносить базу данных.
- `--skip-files` — не переносить файлы.
- `--no-start` — не запускать целевой контур после миграции.

Важно:

- миграция заменяет целевую БД;
- миграция очищает и заменяет целевое файловое хранилище;
- перед использованием в проде стоит иметь свежий бэкап самого `prod`.

## Часовой пояс

По умолчанию проект настроен на:

```text
Europe/Moscow
```

Он задан:

- в env-шаблонах как `ESPO_TIME_ZONE=Europe/Moscow`;
- в PHP-конфиге как `date.timezone=Europe/Moscow`.

## Git и приватные данные

В репозитории уже предусмотрен `.gitignore`, который скрывает:

- рабочие `.env`-файлы;
- каталоги `storage/`;
- каталоги `backups/`;
- архивы, дампы, логи и прочие локальные артефакты.

В git следует коммитить:

- код;
- шаблоны env-файлов;
- документацию;
- инфраструктурные конфиги;
- shell-скрипты.

Не следует коммитить:

- реальные пароли;
- рабочие `.env.prod` и `.env.dev`;
- бэкапы;
- данные приложения;
- локальные IDE-файлы.

## Важные замечания

- `docker compose down` не удаляет bind-mount каталоги с данными.
- `docker compose down -v` использовать не следует без полного понимания последствий.
- `./scripts/docker-cleanup.sh` специально не удаляет Docker volumes и не заменяет осознанную ручную работу с томами.
- Обновление версии EspoCRM лучше делать осознанно, через смену `ESPOCRM_IMAGE`.
- Лимиты ресурсов и ротация логов задаются через `.env.prod` и `.env.dev`, а не зашиваются прямо в `compose.yaml`.
- Для автозапуска бэкапов можно использовать не только `cron`, но и готовые unit-файлы из `ops/systemd/`.
- Перед публикацией изменений и перед продовым обновлением удобно проходить чеклист из `ops/RELEASE.md`.
- Изменяющие состояние операции (`backup`, `restore`, `migrate`, `update`) защищены lock-файлом в `backups/<контур>/locks/`, чтобы не пересекаться между собой.
- Служебные отчеты и support bundle очищаются автоматически по срокам хранения из env-файлов.

## Лицензия

Проект распространяется по лицензии MIT.

Полный текст лицензии находится в файле `LICENSE`.
