# Приёмочный Корпус: `restore` для `v2`

## Назначение

Этот документ фиксирует приёмочный корпус для первого `restore v2` slice.

Он следует [V2_SCOPE.md](/home/febinet/code/docker/V2_SCOPE.md):

- `v1` используется только как spec harness, regression oracle и emergency patch lane
- `v2` не повторяет внутреннюю архитектуру `v1`
- `v2` сохраняет корректное наблюдаемое поведение `v1`, кроме legacy-семантики, которая противоречит жёстким инвариантам `V2_SCOPE.md`

Корпус проверяет product behavior:

- CLI surface текущей команды `restore` как black-box reference
- internal `restore v2` машинный contract
- disk/runtime post-conditions
- наблюдаемые side effects аварийного snapshot
- fail-closed behavior на destructive path
- отсутствие ложного success

На этом slice cutover CLI не выполняется.

## Источник истины

Первый источник истины для `restore v2`:

- выбор restore source
- машинный result contract
- snapshot/no-snapshot семантика
- runtime stop/return/no-stop/no-start семантика
- DB restore и files restore post-conditions
- согласование прав как отдельный наблюдаемый step
- fail-closed семантика для source/snapshot/db/files/permission/runtime errors

Точные human-readable строки не являются invariant-контрактом.
Новые UI/docs/comments в `v2` ведутся на русском, поэтому английские строки `v1` фиксируются только как legacy reference.

## Жёсткие Инварианты

- manifest существует только как complete backup-set contract
- частичный restore не использует manifest как нормальный product contract
- db-only restore идёт только через direct DB artifact + `--skip-files`
- files-only restore идёт только через direct files artifact + `--skip-db`
- destructive path всегда fail-closed
- success сообщается только после явных restore post-checks
- adapters не принимают policy decisions
- policy живёт выше runtime/store
- runtime/store только исполняют выбранный app policy path

## Legacy Behavior Из `v1`

`v1` может принимать partial manifest для части restore/migrate paths.
Для `restore v2` это не становится нормальным контрактом.

Legacy facts, которые фиксируются как reference, но не поднимаются до `v2` invariant:

- partial-manifest семантика
- transport quirks на некоторых failure paths
- exact English strings
- форма внутренних `v1` flow packages

## Минимальный Машинный Contract

Для internal `restore v2` result первого slice закрепляет:

- `command == "restore"`
- `ok == true|false`
- `process_exit_code`
- `details.ready`
- `details.scope`
- `details.selection_mode`
- `details.source_kind`
- `details.snapshot_enabled`
- `details.skip_db`
- `details.skip_files`
- `details.no_stop`
- `details.no_start`
- `details.app_services_were_running`
- counters `steps`, `completed`, `skipped`, `blocked`, `failed`, `warnings`
- `artifacts.project_dir`
- `artifacts.compose_file`
- `artifacts.env_file`
- `artifacts.backup_root`
- `artifacts.manifest_json`
- `artifacts.db_backup`
- `artifacts.files_backup`
- `artifacts.snapshot_manifest_json`
- `artifacts.snapshot_db_backup`
- `artifacts.snapshot_files_backup`
- `items[*].code`
- `items[*].status`
- при failure: `error.kind`, `error.code`, `error.exit_code`

Точная формулировка `message`, `summary`, `details`, `action` не фиксируется как invariant.

## Каталог Сценариев

Статусы источника:

- `подтверждено v1`: black-box reference доступен через текущий CLI path
- `internal v2`: покрывается первым internal implementation slice без CLI cutover
- `legacy divergence`: `v1` behavior зафиксирован, но не переносится в `v2`
- `нужно доснять v1`: требуется отдельный black-box reference перед cutover

### 1. Успешные Сценарии

- `RST-001` Полный restore по manifest.
  Ожидается:
  source берётся из complete manifest; выполняются snapshot, runtime prepare, DB restore, files restore, согласование прав, runtime return и post-check; success только после post-check.
  Статус: `подтверждено v1`, `internal v2`.

- `RST-002` Полный restore по direct pair.
  Ожидается:
  DB/files artifacts проверяются напрямую; artifacts принадлежат одному backup-set; manifest не требуется; полный restore завершается после post-check.
  Статус: `подтверждено v1`, `internal v2`.

- `RST-101` DB-only restore.
  Ожидается:
  источник только direct DB artifact; `--skip-files=true`; files restore и согласование прав не выполняются.
  Статус: `подтверждено v1`, `internal v2`.

- `RST-102` Files-only restore.
  Ожидается:
  источник только direct files artifact; `--skip-db=true`; DB restore не выполняется; files restore и согласование прав выполняются.
  Статус: `подтверждено v1`, `internal v2`.

### 2. Usage И Ошибки Выбора Источника

- `RST-201` Usage error: source не задан.
  Ожидается:
  mutating path не стартует; success не сообщается.
  Статус: `подтверждено v1`.

- `RST-202` Usage error: manifest и direct inputs заданы одновременно.
  Ожидается:
  mutating path не стартует; success не сообщается.
  Статус: `подтверждено v1`.

- `RST-203` Usage error: одновременно `--skip-db` и `--skip-files`.
  Ожидается:
  mutating path не стартует; success не сообщается.
  Статус: `подтверждено v1`.

- `RST-204` Invalid direct source combination.
  Ожидается:
  direct pair неполон или artifacts относятся к разным backup-set; fail closed.
  Статус: `подтверждено v1`, `internal v2`.

- `RST-205` Partial restore через manifest.
  Ожидается:
  fail closed; partial manifest/manifest+skip не является нормальным `v2` contract.
  Статус: `legacy divergence`, `internal v2`.

### 3. Snapshot Semantics

- `RST-301` Normal pre-restore snapshot.
  Ожидается:
  snapshot создаётся до destructive restore; snapshot artifacts видны в machine result.
  Статус: `подтверждено v1`, `internal v2`.

- `RST-302` `--no-snapshot`.
  Ожидается:
  snapshot step skipped; snapshot artifacts отсутствуют; restore может продолжаться.
  Статус: `подтверждено v1`, `internal v2`.

- `RST-303` Snapshot failure.
  Ожидается:
  DB/files restore не выполняются; runtime return выполняется, если runtime уже менялся; success не сообщается.
  Статус: `подтверждено v1`, `internal v2`.

### 4. Runtime Semantics

- `RST-401` Runtime stop/start по умолчанию.
  Ожидается:
  application services останавливаются перед restore и возвращаются после restore; post-check подтверждает готовность.
  Статус: `подтверждено v1`, `internal v2`.

- `RST-402` `--no-stop`.
  Ожидается:
  application services не останавливаются; result явно отражает `no_stop=true`.
  Статус: `подтверждено v1`, `internal v2`.

- `RST-403` `--no-start`.
  Ожидается:
  application services остаются остановленными после restore; success требует явного post-check для оставшихся expected services или явного skipped post-check.
  Статус: `подтверждено v1`, `internal v2`.

- `RST-404` Runtime return failure.
  Ожидается:
  restore не сообщает success, даже если DB/files уже восстановлены.
  Статус: `подтверждено v1`, `internal v2`.

### 5. Ошибки Restore

- `RST-501` DB restore failure.
  Ожидается:
  files restore не выполняется; runtime return не теряется; success не сообщается.
  Статус: `подтверждено v1`, `internal v2`.

- `RST-502` Files restore failure.
  Ожидается:
  согласование прав не выполняется; runtime return не теряется; success не сообщается.
  Статус: `подтверждено v1`, `internal v2`.

- `RST-503` Permission reconciliation failure.
  Ожидается:
  files уже могли быть восстановлены, но overall restore failed; runtime return не теряется.
  Статус: `подтверждено v1`, `internal v2`.

- `RST-504` Invalid manifest.
  Ожидается:
  fail closed до runtime mutation.
  Статус: `подтверждено v1`, `internal v2`.

- `RST-505` Missing artifact.
  Ожидается:
  fail closed до runtime mutation.
  Статус: `подтверждено v1`, `internal v2`.

- `RST-506` Broken archive.
  Ожидается:
  fail closed; success не сообщается.
  Статус: `подтверждено v1`, `internal v2`.

- `RST-507` Checksum mismatch.
  Ожидается:
  fail closed до runtime mutation.
  Статус: `подтверждено v1`, `internal v2`.

### 6. Нужно Доснять Из `v1` Перед Cutover

- полный CLI JSON/error envelope для `--no-stop`
- полный CLI JSON/error envelope для `--no-start`
- runtime return failure envelope
- согласование прав failure envelope
- snapshot failure disk/runtime post-conditions
- partial manifest behavior как legacy divergence reference

## Первый Internal Slice

Первый internal slice закрывает:

- `RST-001`
- `RST-002`
- `RST-101`
- `RST-102`
- `RST-204`
- `RST-205`
- `RST-301`
- `RST-302`
- `RST-303`
- `RST-401`
- `RST-402`
- `RST-403`
- `RST-404`
- `RST-501`
- `RST-502`
- `RST-503`
- `RST-504`
- `RST-505`
- `RST-506`
- `RST-507`

`RST-201`, `RST-202`, `RST-203` остаются CLI usage reference до cutover wiring.
На этом шаге real CLI `restore` не переключается на `v2`.
