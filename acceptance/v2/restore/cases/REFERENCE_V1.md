# `v1` Reference Bundles для `restore`

Этот файл связывает `RST-*` с воспроизводимыми black-box reference bundles из `v1`.

Источник генерации:

- [internal/cli/restore_acceptance_reference_test.go](/home/febinet/code/docker/internal/cli/restore_acceptance_reference_test.go)

Обновление конкретного bundle:

```bash
UPDATE_ACCEPTANCE_RESTORE_REFERENCE=1 go test ./internal/cli -run 'TestAcceptanceReference_RestoreV1_JSONDiskAndRuntime/RST-402$' -count=1
```

Проверка без перегенерации:

```bash
go test ./internal/cli -run 'TestAcceptanceReference_RestoreV1_JSONDiskAndRuntime/RST-402$' -count=1
```

## Что Фиксируется

- полный CLI JSON/error envelope текущего `restore` path
- `process exit code`
- disk post-conditions по `storage` и `backup_root`
- runtime post-conditions по running services и docker log
- observable snapshot artifacts там, где они реально появились
- legacy transport quirks и step layout только как reference facts

## Что Не Становится Инвариантом `v2`

- exact English UI strings
- exact `summary` / `details` / `action` phrasing
- legacy item grouping внутри `v1` envelope
- legacy partial-manifest semantics
- transport quirks, если они не входят в machine contract и observable semantics из `ACCEPTANCE.md`

## Bundles

- `RST-205`
  Подтверждает: `RST-205`
  JSON: [v1_RST-205.json](/home/febinet/code/docker/acceptance/v2/restore/golden/json/v1_RST-205.json)
  Disk: [v1_RST-205.json](/home/febinet/code/docker/acceptance/v2/restore/golden/disk/v1_RST-205.json)
  Runtime: [v1_RST-205.json](/home/febinet/code/docker/acceptance/v2/restore/golden/runtime/v1_RST-205.json)
  Примечание: `v1` принимает partial manifest + `--skip-files` как `manifest_db_only` success. Это legacy divergence reference и не становится нормальным `v2` contract.

- `RST-303`
  Подтверждает: `RST-303`
  JSON: [v1_RST-303.json](/home/febinet/code/docker/acceptance/v2/restore/golden/json/v1_RST-303.json)
  Disk: [v1_RST-303.json](/home/febinet/code/docker/acceptance/v2/restore/golden/disk/v1_RST-303.json)
  Runtime: [v1_RST-303.json](/home/febinet/code/docker/acceptance/v2/restore/golden/runtime/v1_RST-303.json)
  Примечание: при failure аварийного snapshot `v1` блокирует DB/files restore и не выполняет runtime return; после команды запущенным остаётся только `db`.

- `RST-402`
  Подтверждает: `RST-402`
  JSON: [v1_RST-402.json](/home/febinet/code/docker/acceptance/v2/restore/golden/json/v1_RST-402.json)
  Disk: [v1_RST-402.json](/home/febinet/code/docker/acceptance/v2/restore/golden/disk/v1_RST-402.json)
  Runtime: [v1_RST-402.json](/home/febinet/code/docker/acceptance/v2/restore/golden/runtime/v1_RST-402.json)
  Примечание: `v1` не останавливает app services перед restore, но success envelope всё равно проходит через runtime-return path и `compose up -d`.

- `RST-403`
  Подтверждает: `RST-403`
  JSON: [v1_RST-403.json](/home/febinet/code/docker/acceptance/v2/restore/golden/json/v1_RST-403.json)
  Disk: [v1_RST-403.json](/home/febinet/code/docker/acceptance/v2/restore/golden/disk/v1_RST-403.json)
  Runtime: [v1_RST-403.json](/home/febinet/code/docker/acceptance/v2/restore/golden/runtime/v1_RST-403.json)
  Примечание: `v1` оставляет application services остановленными и подтверждает health только для `db`, но транспортно всё ещё показывает completed runtime-return item.

- `RST-404`
  Подтверждает: `RST-404`
  JSON: [v1_RST-404.json](/home/febinet/code/docker/acceptance/v2/restore/golden/json/v1_RST-404.json)
  Disk: [v1_RST-404.json](/home/febinet/code/docker/acceptance/v2/restore/golden/disk/v1_RST-404.json)
  Runtime: [v1_RST-404.json](/home/febinet/code/docker/acceptance/v2/restore/golden/runtime/v1_RST-404.json)
  Примечание: DB/files side effects уже на диске, но overall result failure; app services после failed runtime return остаются остановленными.

- `RST-503`
  Подтверждает: `RST-503`
  JSON: [v1_RST-503.json](/home/febinet/code/docker/acceptance/v2/restore/golden/json/v1_RST-503.json)
  Disk: [v1_RST-503.json](/home/febinet/code/docker/acceptance/v2/restore/golden/disk/v1_RST-503.json)
  Runtime: [v1_RST-503.json](/home/febinet/code/docker/acceptance/v2/restore/golden/runtime/v1_RST-503.json)
  Примечание: files уже восстановлены, но permission reconcile ломается внутри failure envelope; runtime return блокируется; observed file modes остаются unreconciled (`0755/0644` вместо `0775/0664` под `data/custom/client/custom/upload`).
