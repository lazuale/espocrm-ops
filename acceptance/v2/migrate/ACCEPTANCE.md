# Приёмочный Корпус: `migrate` для `v2`

## Назначение

Этот документ фиксирует acceptance-first corpus для `migrate v2`.

Он следует [V2_SCOPE.md](/home/febinet/code/docker/V2_SCOPE.md):

- `v1` используется только как spec harness, regression oracle и emergency patch lane
- `v2` не повторяет внутреннюю форму legacy `migrate`
- real CLI path `migrate` переключён на retained core через `internal/app`
- manifest остаётся только complete backup-set contract

Корпус проверяет:

- source selection и compatibility policy
- target snapshot и destructive migrate path
- runtime stop/start/return semantics
- disk/runtime post-conditions
- fail-closed поведение
- отсутствие ложного success

## Источник Истины

Источник истины для retained `migrate v2` corpus:

- текущий CLI `migrate` path как black-box reference там, где legacy surface уже что-то наблюдаемо фиксирует
- internal `migrate v2` machine contract для нового execution core
- acceptance bundles по JSON, disk и runtime post-conditions

Точные human-readable строки `v1` не являются invariant-контрактом.
Новые docs/comments/result notes в `v2` ведутся на русском.

## Жёсткие Инварианты

- manifest существует только как complete backup-set contract
- partial migrate через manifest не становится нормальным `v2` contract
- direct partial migrate идёт только через direct artifacts
- полный migrate не выводит недостающий artifact из одного explicit artifact
- source-selection policy живёт в `app/model`
- compatibility policy живёт в `app/model`
- adapters только исполняют выбранный workflow
- destructive path всегда fail-closed
- success сообщается только после явного post-check
- target snapshot создаётся до destructive apply в `v2`

## Legacy Facts Из `v1`

Legacy facts, которые фиксируются только как reference, но не становятся обязательным `v2` invariant:

- exact English strings
- legacy step phrasing и envelope quirks
- implicit pairing из одного explicit artifact, если оно не входит в retained `v2` contract
- отсутствие target snapshot в test-only legacy `migrate` harness

## Минимальный Машинный Contract

Для retained `migrate v2` corpus закрепляется:

- `command == "migrate"`
- `ok == true|false`
- `process_exit_code`
- `details.ready`
- `details.source_scope`
- `details.target_scope`
- `details.selection_mode`
- `details.source_kind`
- `details.snapshot_enabled`
- `details.skip_db`
- `details.skip_files`
- `details.no_start`
- `details.app_services_were_running`
- `details.started_db_temporarily`
- counters `steps`, `completed`, `skipped`, `blocked`, `failed`, `warnings`
- `artifacts.project_dir`
- `artifacts.compose_file`
- `artifacts.source_env_file`
- `artifacts.target_env_file`
- `artifacts.source_backup_root`
- `artifacts.target_backup_root`
- `artifacts.manifest_json`
- `artifacts.db_backup`
- `artifacts.files_backup`
- `artifacts.snapshot_manifest_json`
- `artifacts.snapshot_db_backup`
- `artifacts.snapshot_files_backup`
- `items[*].code`
- `items[*].status`
- при failure: `error.kind`, `error.code`, `error.exit_code`

Точная phrasing полей `message`, `summary`, `details`, `action` не фиксируется как invariant.

## Каталог Сценариев

Статусы источника:

- `подтверждено v1`: black-box reference bundle снят с test-only legacy `migrate` harness
- `internal v2`: покрывается internal implementation corpus
- `legacy divergence`: зафиксировано как legacy fact и не поднимается в `v2` invariant
- `отложено`: не входит в retained corpus и остаётся на отдельный parity/cutover шаг

### 1. Успешные Сценарии

- `MIG-001` Полный migrate из latest complete backup-set.
  Ожидается:
  source автоматически выбирается как latest complete verified set; target snapshot выполняется до destructive apply; затем идут runtime prepare, DB/files apply, permission reconcile, runtime return и post-check.
  Статус: `подтверждено v1`, `internal v2`, `real CLI v2`.

- `MIG-002` Полный migrate по direct pair.
  Ожидается:
  explicit DB/files artifacts проверяются напрямую; artifacts принадлежат одному backup-set; migrate завершается только после post-check.
  Статус: `подтверждено v1`, `internal v2`, `real CLI v2`.

- `MIG-101` DB-only migrate.
  Ожидается:
  источник только direct DB artifact; `--skip-files=true`; files apply и permission reconcile не выполняются; target snapshot берётся только по DB-части destructive path.
  Статус: `подтверждено v1`, `internal v2`, `real CLI v2`.

- `MIG-102` Files-only migrate.
  Ожидается:
  источник только direct files artifact; `--skip-db=true`; DB apply не выполняется; files apply и permission reconcile выполняются; target snapshot берётся только по files-части destructive path.
  Статус: `подтверждено v1`, `internal v2`, `real CLI v2`.

### 2. Usage И Ошибки Выбора Источника

- `MIG-201` Usage error: `--force` обязателен.
  Ожидается:
  mutating path не стартует; success не сообщается.
  Статус: `подтверждено v1`, `real CLI v2`.

- `MIG-202` Usage error: prod target требует `--confirm-prod prod`.
  Ожидается:
  mutating path не стартует; success не сообщается.
  Статус: `подтверждено v1`, `real CLI v2`.

- `MIG-203` Usage error: source и target contour совпадают.
  Ожидается:
  mutating path не стартует; success не сообщается.
  Статус: `подтверждено v1`, `real CLI v2`.

- `MIG-204` Usage error: одновременно `--skip-db` и `--skip-files`.
  Ожидается:
  mutating path не стартует; success не сообщается.
  Статус: `подтверждено v1`, `real CLI v2`.

- `MIG-205` Invalid matching manifest blocks latest complete selection.
  Ожидается:
  если matching manifest для выбранного complete set существует, но incoherent или invalid, automatic source selection не проходит; destructive path не стартует.
  Статус: `подтверждено v1`, `internal v2`, `real CLI v2`.

- `MIG-206` Invalid direct pair combination.
  Ожидается:
  explicit DB/files artifacts относятся к разным backup-set; migrate fail closed до target snapshot/runtime mutation.
  Статус: `подтверждено v1`, `internal v2`, `real CLI v2`.

- `MIG-207` Full migrate с implicit pairing от explicit DB artifact.
  Ожидается:
  legacy CLI может вывести matching files artifact из stamp explicit DB backup, но `migrate v2` не принимает такой source-selection path и блокирует его fail-closed до target snapshot/runtime mutation.
  Статус: `подтверждено v1`, `legacy divergence`, `internal v2`, `real CLI v2`.

- `MIG-208` Full migrate с implicit pairing от explicit files artifact.
  Ожидается:
  legacy CLI может вывести matching DB artifact из stamp explicit files backup, но `migrate v2` не принимает такой source-selection path и блокирует его fail-closed до target snapshot/runtime mutation.
  Статус: `подтверждено v1`, `legacy divergence`, `internal v2`, `real CLI v2`.

### 3. Compatibility Failures

- `MIG-301` Compatibility drift.
  Ожидается:
  governed source/target settings не совпадают; destructive path не стартует; success не сообщается.
  Статус: `подтверждено v1`, `internal v2`, `real CLI v2`.

### 4. Snapshot И Runtime Semantics

- `MIG-401` Target snapshot before destructive path.
  Ожидается:
  `v2` делает target snapshot до runtime prepare и destructive apply; snapshot artifacts попадают в machine result.
  Статус: `internal v2`, `real CLI v2`.
  Примечание: test-only legacy `migrate` harness не имеет отдельного target snapshot behavior, поэтому `v1` reference здесь отсутствует.

- `MIG-402` `--no-start`.
  Ожидается:
  после успешного migrate application services остаются остановленными; success возможен только после post-check для требуемого подмножества runtime.
  Статус: `подтверждено v1`, `internal v2`, `real CLI v2`.

- `MIG-403` Runtime return или post-check failure.
  Ожидается:
  destructive side effects уже могли произойти, но success не сообщается; runtime post-condition остаётся fail-closed.
  Статус: `подтверждено v1`, `internal v2`, `real CLI v2`.

### 5. Ошибки Destructive Path

- `MIG-501` Target snapshot failure.
  Ожидается:
  destructive apply не стартует; runtime prepare не стартует; success не сообщается.
  Статус: `internal v2`, `real CLI v2`.

- `MIG-502` DB migrate failure.
  Ожидается:
  files apply не выполняется; runtime return блокируется; success не сообщается.
  Статус: `internal v2`, `real CLI v2`.

- `MIG-503` Files migrate failure.
  Ожидается:
  permission reconcile не выполняется; runtime return блокируется; success не сообщается.
  Статус: `internal v2`, `real CLI v2`.

- `MIG-504` Permission reconciliation failure.
  Ожидается:
  files уже могли быть восстановлены на disk, но runtime permission reconciliation ломается; runtime return блокируется; target app services остаются остановленными.
  Статус: `подтверждено v1`, `internal v2`, `real CLI v2`.

- `MIG-505` Missing artifact.
  Ожидается:
  fail closed до destructive runtime mutation.
  Статус: `internal v2`, `real CLI v2`.

- `MIG-506` Broken archive.
  Ожидается:
  fail closed; success не сообщается.
  Статус: `internal v2`, `real CLI v2`.

- `MIG-507` Checksum mismatch.
  Ожидается:
  fail closed до destructive runtime mutation.
  Статус: `internal v2`, `real CLI v2`.

### 6. Legacy Divergences

- `MIG-601` Partial manifest semantics не становятся `migrate v2` contract.
  Ожидается:
  manifest остаётся complete backup-set metadata contract; partial migrate через manifest не поднимается в нормальный `v2` product path.
  Статус: `legacy divergence`.

- `MIG-602` Exact legacy strings и transport quirks.
  Ожидается:
  фиксируются только как reference facts и не становятся invariant.
  Статус: `legacy divergence`.

## Internal Corpus

Internal corpus закрывает:

- automatic latest complete source selection
- direct pair / db-only / files-only source selection
- fail-closed rejection для legacy implicit pairing от одного explicit artifact
- matching manifest validation для complete source selection
- compatibility evaluation
- target snapshot до destructive apply
- runtime prepare target contour
- DB/files migrate apply
- permission reconciliation
- runtime return
- post-check и machine-readable result contract

На этом шаге не закрываются:

- любые новые flags и режимы

Real CLI cutover lane дополнительно закрывает:

- real CLI boundary через один `internal/app` owner
- fail-closed запрет legacy selection routes на command surface
- `MIG-201`, `MIG-202`, `MIG-203`, `MIG-204` на реальном command path
- `MIG-205`, `MIG-206`, `MIG-401`, `MIG-501`, `MIG-502`, `MIG-503`, `MIG-505`, `MIG-506`, `MIG-507` на реальном command path
- cutover acceptance lane для реального `migrate` CLI
- удаление legacy `migrate` path из production graph; legacy остаётся только как test-only oracle

## Reference Material

Black-box reference bundles из test-only legacy `migrate` harness описаны в [acceptance/v2/migrate/cases/REFERENCE_V1.md](/home/febinet/code/docker/acceptance/v2/migrate/cases/REFERENCE_V1.md).

Real CLI cutover bundles живут в `acceptance/v2/migrate/cutover/golden/`.

## Статус После Real CLI Cutover

Уже подтверждены `v1` bundles:

- `MIG-001`
- `MIG-002`
- `MIG-101`
- `MIG-102`
- `MIG-201`
- `MIG-202`
- `MIG-203`
- `MIG-204`
- `MIG-205`
- `MIG-206`
- `MIG-207`
- `MIG-208`
- `MIG-301`
- `MIG-402`
- `MIG-403`
- `MIG-504`

Уже покрыты internal `v2` golden/runtime/disk acceptance tests:

- `MIG-001`
- `MIG-002`
- `MIG-101`
- `MIG-102`
- `MIG-205`
- `MIG-206`
- `MIG-207`
- `MIG-208`
- `MIG-301`
- `MIG-401`
- `MIG-402`
- `MIG-403`
- `MIG-501`
- `MIG-502`
- `MIG-503`
- `MIG-504`
- `MIG-505`
- `MIG-506`
- `MIG-507`

Уже покрыты real CLI cutover acceptance tests:

- `MIG-001`
- `MIG-002`
- `MIG-101`
- `MIG-102`
- `MIG-201`
- `MIG-202`
- `MIG-203`
- `MIG-204`
- `MIG-205`
- `MIG-206`
- `MIG-207`
- `MIG-208`
- `MIG-301`
- `MIG-401`
- `MIG-402`
- `MIG-403`
- `MIG-501`
- `MIG-502`
- `MIG-503`
- `MIG-504`
- `MIG-505`
- `MIG-506`
- `MIG-507`

Real CLI cutover corpus теперь закрывает все сценарии, которые раньше были доказаны только internal acceptance в текущем retained `migrate` наборе.

## Статус Slice

Real CLI path переключён на `migrate v2` boundary внутри `internal/app`.
Legacy `migrate` больше не участвует в production wiring и остаётся только test-only oracle для `v1` reference bundles.
