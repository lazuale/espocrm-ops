# Acceptance Corpus: `backup verify` для `v2`

## Назначение

Этот документ фиксирует первый acceptance corpus для команды `backup verify` в `v2`.

Он следует [V2_SCOPE.md](/home/febinet/code/docker/V2_SCOPE.md):

- `v1` используется только как spec harness, regression oracle и emergency patch lane
- `v2` не повторяет внутреннюю архитектуру `v1`
- `v2` сохраняет корректное наблюдаемое поведение `v1`, кроме legacy-семантики, которая противоречит жёстким инвариантам `V2_SCOPE.md`

Corpus проверяет только product behavior:

- CLI behavior текущей command surface `backup verify`
- machine-readable JSON contract
- disk artifacts и отсутствие мутаций во время verify
- manifest/checksum/archive semantics
- fail-closed behavior
- observable post-conditions

## Источник истины

Первый source of truth для `backup verify v2`:

- exit status
- error code и error kind
- структура machine-readable JSON-result
- выбранный manifest для `--backup-root`
- проверка manifest-backed complete backup-set
- проверка direct DB/files artifacts внутри v2 core
- checksum sidecar semantics
- читаемость `.sql.gz` и `.tar.gz`
- отсутствие новых artifacts после verify

Exact human-readable strings не являются invariant-контрактом первого corpus.
Новые UI/docs/comments в `v2` ведутся на русском, поэтому английские строки из `v1` фиксируются только как legacy reference.

## Legacy behavior из `v1`, которое нельзя переносить автоматически

`v1` допускает partial manifest как источник для некоторых restore/migrate paths.
Для `backup verify v2` это не становится нормальным контрактом:

- manifest существует только как complete backup-set contract
- partial backup представлен direct artifact + checksum sidecar
- partial manifest должен fail closed для `backup verify v2`

Текущая CLI surface `backup verify` имеет только:

- `--manifest`
- `--backup-root`

Direct DB/files verify закрепляется в первом `v2` core slice как внутренний product contract для уже существующих direct artifacts.
Новые CLI flags для direct verify в этом срезе не добавляются, потому что это расширило бы product surface.

Если `v1` показывает transport quirks или legacy envelope shape, это фиксируется в reference, но не поднимается автоматически до обязательного invariant-контракта `v2`.

## Минимальный machine-readable contract

Для `v2` JSON-result первого slice закрепляет:

- `command == "backup verify"`
- `ok == true|false`
- `process_exit_code`
- `details.ready`
- `details.source_kind`
- `details.scope`
- `details.created_at`
- counters `steps`, `completed`, `skipped`, `blocked`, `failed`
- `artifacts.backup_root`
- `artifacts.manifest`
- `artifacts.db_backup`
- `artifacts.db_checksum`
- `artifacts.files_backup`
- `artifacts.files_checksum`
- `items[*].code`
- `items[*].status`
- при failure: `error.kind`, `error.code`, `error.exit_code`

Exact wording `message` / `error.message` / text UI не фиксируется как invariant.

## Каталог сценариев

Статусы источника:

- `подтверждено v1`: доступен black-box reference через текущий CLI path
- `подтверждено v2 backup`: артефакты создаются `backup v2`, verify semantics проверяются поверх этих артефактов
- `core-only`: сценарий есть в product semantics, но текущая CLI surface не имеет отдельного флага без расширения surface
- `legacy divergence`: `v1` behavior зафиксирован, но не переносится в `v2`

### 1. Happy Path

- `BKV-001` Verify полного backup-set по manifest.
  Ожидается:
  success; manifest валиден как complete backup-set; DB/files artifacts существуют, читаются, checksum sidecar существует и совпадает с manifest checksum.
  Статус источника: `подтверждено v1`, `подтверждено v2 backup`.

- `BKV-002` Verify latest complete set по backup root.
  Ожидается:
  success; выбирается latest полностью проверяемый complete set из `manifests/*.manifest.json`; incomplete/non-complete candidates не становятся success-контрактом.
  Статус источника: `подтверждено v1`, `подтверждено v2 backup`.

- `BKV-101` Verify direct DB backup.
  Ожидается:
  success; `.sql.gz` существует, не пустой, gzip читается, checksum sidecar существует и совпадает.
  Статус источника: `core-only`.

- `BKV-102` Verify direct files backup.
  Ожидается:
  success; `.tar.gz` существует, не пустой, archive читается, checksum sidecar существует и совпадает.
  Статус источника: `core-only`.

### 2. Usage/Selection Failures

- `BKV-201` Usage error: не задан ни `--manifest`, ни `--backup-root`.
  Ожидается:
  usage error до verify path; disk не меняется.
  Статус источника: `подтверждено v1`.

- `BKV-202` Usage error: одновременно заданы `--manifest` и `--backup-root`.
  Ожидается:
  usage error до verify path; disk не меняется.
  Статус источника: `подтверждено v1`.

- `BKV-401` Backup root не содержит проверяемого complete set.
  Ожидается:
  fail closed; success не сообщается; disk не меняется.
  Статус источника: `подтверждено v1`.

### 3. Manifest Failures

- `BKV-301` Manifest невозможно прочитать/распарсить/валидировать.
  Ожидается:
  fail closed; `manifest_invalid`; disk не меняется.
  Статус источника: `подтверждено v1`.

- `BKV-305` Partial manifest не считается нормальным `v2` контрактом.
  Ожидается:
  fail closed; manifest-backed success не сообщается.
  Статус источника: `legacy divergence`.

### 4. Artifact/Checksum Failures

- `BKV-302` Checksum mismatch.
  Ожидается:
  fail closed; success не сообщается; disk не меняется.
  Статус источника: `подтверждено v1`, `подтверждено v2 backup`.

- `BKV-303` Archive unreadable/corrupted.
  Ожидается:
  fail closed; success не сообщается; disk не меняется.
  Статус источника: `подтверждено v1`, `подтверждено v2 backup`.

- `BKV-304` Artifact из manifest отсутствует.
  Ожидается:
  fail closed; success не сообщается; disk не меняется.
  Статус источника: `подтверждено v1`, `подтверждено v2 backup`.

### 5. Non-Goals Для Первого Slice

- cutover текущего CLI path на `backup verify v2`
- новые flags для direct DB/files verify
- `restore`
- `migrate`
- `doctor`
- перенос legacy partial-manifest semantics
- новый doctrinal test layer
- глобальный runtime/preflight contract для остальных команд

## Первый implementation slice

Минимальный первый slice закрывает:

- `BKV-001`
- `BKV-002`
- `BKV-101`
- `BKV-102`
- `BKV-301`
- `BKV-302`
- `BKV-303`
- `BKV-304`
- `BKV-305`
- `BKV-401`

`BKV-201` и `BKV-202` остаются покрытыми текущими CLI-level tests как reference для existing command surface.
Cutover-safe wiring для CLI выполняется только после parity review по этому corpus.

## Cutover-Safe Wiring Slice

После internal parity real CLI path `backup verify` переключается на `backup verify v2` только для существующей surface:

- `--manifest`
- `--backup-root`

Новые direct DB/files CLI flags не добавляются.
Direct DB/files verify остаётся core-only до отдельного product-surface решения.

Через реальный CLI path должны проходить:

- `BKV-001`
- `BKV-002`
- `BKV-201`
- `BKV-202`
- `BKV-301`
- `BKV-302`
- `BKV-303`
- `BKV-304`
- `BKV-401`

Legacy facts, которые не поднимаются до `v2` invariant:

- failure envelope `command == "espops"` на некоторых `v1` failure paths
- partial-manifest semantics
- exact English strings

## Контролируемое Удаление

После cutover реальный CLI path `backup verify` обслуживается только `backup verify v2`.
Старый command-specific app path удаляется, потому что он больше не участвует в выполнении команды.

Сознательно сохраняются:

- `v1_BKV-*` golden/reference bundles как oracle-only материал
- общие `backupstore` manifest/checksum helpers, пока они используются `restore` / `migrate`
- legacy store adapter методы, пока они нужны retained commands вне `backup verify`

Старые transport-quirks из `v1` не возвращаются как обязательный `v2` contract.
