# Release-памятка

Этот файл нужен как короткий регламент перед публикацией изменений в GitHub или перед ручным обновлением продового окружения.

## Перед коммитом

Проверьте:

```bash
./scripts/doctor.sh all
./scripts/smoke-test.sh dev --from-example
```

Если стек уже запущен, дополнительно полезно:

```bash
./scripts/backup.sh prod
./scripts/verify-backup.sh prod
./scripts/backup-audit.sh prod
./scripts/backup-catalog.sh prod --ready-only --verify-checksum
./scripts/restore-drill.sh prod
```

## Перед публикацией в Git

Убедитесь, что:

- в репозиторий не попали `.env.prod`, `.env.dev`, `storage/`, `backups/`;
- обновлен `CHANGELOG.md`, если изменения заметные;
- команды из `README.md`, `START.md` и `ops/CHEATSHEET.md` соответствуют текущей реализации;
- `systemd`-шаблоны и `cron.example` тоже синхронизированы, если менялся backup/doctor flow.

## Перед обновлением прода

Рекомендуемая последовательность:

1. Сделать свежий backup прод-контура.
2. Проверить backup через `./scripts/verify-backup.sh prod`.
3. Прогнать `./scripts/backup-audit.sh prod`.
4. Прогнать `./scripts/restore-drill.sh prod`, чтобы убедиться, что backup реально восстанавливается в изолированный контур.
5. Прогнать `./scripts/doctor.sh prod`.
6. Обновить `ESPOCRM_IMAGE` в `.env.prod`, если меняется версия образа.
7. Предпочтительно выполнить `./scripts/update.sh prod`.
8. Если нужен ручной сценарий, вместо этого выполнить `./scripts/stack.sh prod pull` и `./scripts/stack.sh prod up -d`.
9. Проверить `./scripts/stack.sh prod ps`, статус-отчет `./scripts/status-report.sh prod` и логи `./scripts/stack.sh prod logs -f espocrm`.
10. Если обновление прошло с проблемой, сначала собрать `./scripts/support-bundle.sh prod` или использовать автоматически собранный bundle из `backups/prod/support/`.
11. Если нужен быстрый возврат на последнее валидное состояние, выполнить `./scripts/rollback.sh prod`.

## После публикации

- проверить, что CI прошел успешно;
- при необходимости создать git tag;
- обновить внутреннюю документацию или эксплуатационные заметки команды.
