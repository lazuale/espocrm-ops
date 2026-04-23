# `v1` Reference Bundles для `migrate`

Этот файл связывает `MIG-*` с воспроизводимыми black-box reference bundles из current legacy CLI path.

Источник генерации:

- `internal/cli/migrate_acceptance_reference_test.go`

Обновление конкретного bundle:

```bash
UPDATE_ACCEPTANCE_MIGRATE_REFERENCE=1 go test ./internal/cli -run 'TestAcceptanceReference_MigrateV1_JSONDiskAndRuntime/MIG-001$' -count=1
```

Проверка без перегенерации:

```bash
go test ./internal/cli -run 'TestAcceptanceReference_MigrateV1_JSONDiskAndRuntime/MIG-001$' -count=1
```

## Что Фиксируется

- полный CLI JSON/error envelope текущего `migrate` path
- `process exit code`
- disk post-conditions по target storage и source backup root
- runtime post-conditions по running services и docker log
- observable source-selection и compatibility behavior
- legacy transport quirks только как reference facts

## Что Не Становится Инвариантом `v2`

- exact English UI strings
- exact `summary` / `details` / `action` phrasing
- legacy item grouping внутри current CLI envelope
- отсутствие target snapshot в legacy CLI path
- legacy implicit pairing semantics, если они конфликтуют с `V2_SCOPE.md` или не нужны для первого internal slice

## Bundles

- `MIG-001`
  Подтверждает: full migrate из latest complete source selection.
  JSON: `acceptance/v2/migrate/golden/json/v1_MIG-001.json`
  Disk: `acceptance/v2/migrate/golden/disk/v1_MIG-001.json`
  Runtime: `acceptance/v2/migrate/golden/runtime/v1_MIG-001.json`

- `MIG-002`
  Подтверждает: full migrate по explicit direct pair.
  JSON: `acceptance/v2/migrate/golden/json/v1_MIG-002.json`
  Disk: `acceptance/v2/migrate/golden/disk/v1_MIG-002.json`
  Runtime: `acceptance/v2/migrate/golden/runtime/v1_MIG-002.json`

- `MIG-101`
  Подтверждает: DB-only migrate.
  JSON: `acceptance/v2/migrate/golden/json/v1_MIG-101.json`
  Disk: `acceptance/v2/migrate/golden/disk/v1_MIG-101.json`
  Runtime: `acceptance/v2/migrate/golden/runtime/v1_MIG-101.json`

- `MIG-102`
  Подтверждает: files-only migrate.
  JSON: `acceptance/v2/migrate/golden/json/v1_MIG-102.json`
  Disk: `acceptance/v2/migrate/golden/disk/v1_MIG-102.json`
  Runtime: `acceptance/v2/migrate/golden/runtime/v1_MIG-102.json`

- `MIG-205`
  Подтверждает: invalid matching manifest blocks automatic complete source selection.
  JSON: `acceptance/v2/migrate/golden/json/v1_MIG-205.json`
  Disk: `acceptance/v2/migrate/golden/disk/v1_MIG-205.json`
  Runtime: `acceptance/v2/migrate/golden/runtime/v1_MIG-205.json`

- `MIG-206`
  Подтверждает: invalid direct pair combination.
  JSON: `acceptance/v2/migrate/golden/json/v1_MIG-206.json`
  Disk: `acceptance/v2/migrate/golden/disk/v1_MIG-206.json`
  Runtime: `acceptance/v2/migrate/golden/runtime/v1_MIG-206.json`

- `MIG-207`
  Подтверждает: implicit pairing от explicit DB artifact.
  JSON: `acceptance/v2/migrate/golden/json/v1_MIG-207.json`
  Disk: `acceptance/v2/migrate/golden/disk/v1_MIG-207.json`
  Runtime: `acceptance/v2/migrate/golden/runtime/v1_MIG-207.json`
  Примечание: это reference только для legacy behavior; `migrate v2` не принимает такой path как нормальный internal contract.

- `MIG-208`
  Подтверждает: implicit pairing от explicit files artifact.
  JSON: `acceptance/v2/migrate/golden/json/v1_MIG-208.json`
  Disk: `acceptance/v2/migrate/golden/disk/v1_MIG-208.json`
  Runtime: `acceptance/v2/migrate/golden/runtime/v1_MIG-208.json`
  Примечание: это reference только для legacy behavior; `migrate v2` не принимает такой path как нормальный internal contract.

- `MIG-301`
  Подтверждает: compatibility drift blocks destructive path.
  JSON: `acceptance/v2/migrate/golden/json/v1_MIG-301.json`
  Disk: `acceptance/v2/migrate/golden/disk/v1_MIG-301.json`
  Runtime: `acceptance/v2/migrate/golden/runtime/v1_MIG-301.json`

- `MIG-402`
  Подтверждает: successful `--no-start` runtime semantics.
  JSON: `acceptance/v2/migrate/golden/json/v1_MIG-402.json`
  Disk: `acceptance/v2/migrate/golden/disk/v1_MIG-402.json`
  Runtime: `acceptance/v2/migrate/golden/runtime/v1_MIG-402.json`
  Примечание: current CLI path оставляет запущенным только `db`; final `target_start` step transportно помечается `skipped`, а target backup root не получает snapshot artifacts.

- `MIG-403`
  Подтверждает: runtime return / target health failure after destructive apply.
  JSON: `acceptance/v2/migrate/golden/json/v1_MIG-403.json`
  Disk: `acceptance/v2/migrate/golden/disk/v1_MIG-403.json`
  Runtime: `acceptance/v2/migrate/golden/runtime/v1_MIG-403.json`
  Примечание: DB/files side effects уже на target disk, но final health failure ломает overall result; app services после failure не считаются подтверждённо возвращёнными.

- `MIG-504`
  Подтверждает: permission reconciliation failure blocks runtime return.
  JSON: `acceptance/v2/migrate/golden/json/v1_MIG-504.json`
  Disk: `acceptance/v2/migrate/golden/disk/v1_MIG-504.json`
  Runtime: `acceptance/v2/migrate/golden/runtime/v1_MIG-504.json`
  Примечание: files уже восстановлены на disk, но permission reconcile обрывает path до target start; в target backup root observable только locks, а не snapshot set.

- `MIG-401` и `MIG-501`
  Примечание:
  target snapshot semantics не имеют отдельного `v1` reference bundle, потому что current legacy CLI path не создаёт target snapshot как наблюдаемый migrate step.
