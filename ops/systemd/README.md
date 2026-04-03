# systemd-шаблоны

В этой папке лежат готовые примеры unit-файлов для Linux-серверов, где резервное копирование удобнее запускать через `systemd timer`, а не через `cron`.

## Что здесь есть

- `espo-backup@.service` — шаблонный oneshot-сервис для запуска `./scripts/backup.sh` с контуром `%i`
- `espo-backup-prod.timer` — ежедневный backup для `prod`
- `espo-backup-dev.timer` — ежедневный backup для `dev`
- `espo-backup-audit@.service` — шаблонный oneshot-сервис для запуска `./scripts/backup-audit.sh` с контуром `%i`
- `espo-backup-audit-prod.timer` — ежедневный аудит свежести и целостности backup для `prod`
- `espo-backup-audit-dev.timer` — ежедневный аудит свежести и целостности backup для `dev`
- `espo-doctor@.service` — шаблонный oneshot-сервис для запуска `./scripts/doctor.sh` с контуром `%i`
- `espo-doctor-prod.timer` — ежедневная preflight-проверка для `prod`
- `espo-doctor-dev.timer` — ежедневная preflight-проверка для `dev`

## Установка

Скопируйте unit-файлы на сервер:

```bash
sudo cp ops/systemd/espo-backup@.service /etc/systemd/system/
sudo cp ops/systemd/espo-backup-prod.timer /etc/systemd/system/
sudo cp ops/systemd/espo-backup-dev.timer /etc/systemd/system/
sudo cp ops/systemd/espo-backup-audit@.service /etc/systemd/system/
sudo cp ops/systemd/espo-backup-audit-prod.timer /etc/systemd/system/
sudo cp ops/systemd/espo-backup-audit-dev.timer /etc/systemd/system/
sudo cp ops/systemd/espo-doctor@.service /etc/systemd/system/
sudo cp ops/systemd/espo-doctor-prod.timer /etc/systemd/system/
sudo cp ops/systemd/espo-doctor-dev.timer /etc/systemd/system/
```

Затем перечитайте конфигурацию и включите нужные таймеры:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now espo-backup-prod.timer
sudo systemctl enable --now espo-backup-dev.timer
sudo systemctl enable --now espo-backup-audit-prod.timer
sudo systemctl enable --now espo-backup-audit-dev.timer
sudo systemctl enable --now espo-doctor-prod.timer
sudo systemctl enable --now espo-doctor-dev.timer
```

## Проверка

Посмотреть состояние таймеров:

```bash
systemctl status espo-backup-prod.timer
systemctl status espo-backup-dev.timer
systemctl status espo-backup-audit-prod.timer
systemctl status espo-backup-audit-dev.timer
systemctl status espo-doctor-prod.timer
systemctl status espo-doctor-dev.timer
```

Посмотреть расписание:

```bash
systemctl list-timers | grep espo-backup
systemctl list-timers | grep espo-backup-audit
systemctl list-timers | grep espo-doctor
```

Ручной запуск:

```bash
sudo systemctl start espo-backup@prod.service
sudo systemctl start espo-backup@dev.service
sudo systemctl start espo-backup-audit@prod.service
sudo systemctl start espo-backup-audit@dev.service
sudo systemctl start espo-doctor@prod.service
sudo systemctl start espo-doctor@dev.service
```

## Что подстроить под свой сервер

- путь `WorkingDirectory=/opt/espo`, если проект лежит в другом каталоге;
- расписание в `.timer`-файлах;
- пользователя, если нужен не `root`, а отдельный системный пользователь.

Логично держать audit после backup-задач, чтобы таймер проверял уже свежесозданные артефакты.
