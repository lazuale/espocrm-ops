# Acceptance Corpus: `backup` для `v2`

## Назначение

Этот документ фиксирует первый acceptance corpus для команды `backup` в `v2`.

Он следует [V2_SCOPE.md](/home/febinet/code/docker/V2_SCOPE.md):

- `v1` используется только как spec harness, regression oracle и emergency patch lane
- `v2` не обязан повторять внутреннюю форму `v1`
- `v2` обязан повторить корректное наблюдаемое поведение `v1`, кроме случаев, где `v1` противоречит жёстким инвариантам `V2_SCOPE.md`

Этот corpus не проверяет форму кода, пакеты или внутреннюю архитектуру.
Он проверяет только наблюдаемое поведение:

- CLI behavior
- machine-readable JSON contract
- артефакты на диске
- checksum и manifest артефакты
- post-conditions
- ошибки и fail-closed поведение

## Что считается источником истины

Для первого среза `backup` acceptance source of truth такой:

- exit status и error code
- структура JSON-результата и семантика его полей
- наличие или отсутствие артефактов на диске
- читаемость и согласованность backup-артефактов
- post-conditions для runtime и retention
- наблюдаемая последовательность step `code` / `status`, если она не конфликтует с `V2_SCOPE.md`

Что не считается источником истины для первого `v2` corpus:

- точный английский текст UI из `v1`
- точные английские `message`, `summary`, `details`, `warnings`, `action`
- внутренняя сборка workflow внутри `v1`

Причина: новые docs и UI для `v2` ведутся на русском, поэтому exact-string parity с английским UI `v1` не является обязательной.

## Legacy behavior из `v1`, которое нельзя переносить автоматически

В `v1` partial backup (`--skip-db` или `--skip-files`) всё ещё пишет manifest JSON/TXT.

Это противоречит `V2_SCOPE.md`, где зафиксировано:

- manifest существует только как complete backup-set contract
- partial backup представляется только direct artifacts

Поэтому для `v2` это поведение считается legacy behavior и не переносится автоматически.

Правило для acceptance в `v2`:

- полный backup: manifest обязателен
- partial backup: manifest отсутствует, есть только direct artifact и checksum

## Минимальный machine-readable contract для `backup`

Для `--json` initial corpus закрепляет:

- `command == "backup"`
- `ok == true|false`
- для use-case failure: `error.code == "backup_failed"`
- для usage error: CLI остаётся usage error без запуска mutating path
- `details.scope`
- `details.ready`
- `details.created_at`
- `details.skip_db`
- `details.skip_files`
- `details.no_stop`
- `details.consistent_snapshot`
- `details.app_services_were_running`
- `details.retention_days`
- `artifacts.project_dir`
- `artifacts.compose_file`
- `artifacts.env_file`
- `artifacts.backup_root`
- presence/absence для `manifest_txt`, `manifest_json`, `db_backup`, `files_backup`, `db_checksum`, `files_checksum`
- `items[*].code`
- `items[*].status`

Exact wording полей `message`, `summary`, `details`, `warnings`, `action` в initial corpus не фиксируется.

## Каталог сценариев

Ниже перечислены сценарии, которые должны войти в acceptance corpus для `backup v2`.

Статус источника:

- `подтверждено v1`: уже есть пригодный black-box reference в `v1`
- `частично подтверждено v1`: есть reference только для части поведения, остальное нужно доснять
- `нужно доснять из v1`: пригодного black-box reference пока нет, его нужно снять до cutover

### 1. Happy Path

- `BKP-001` Полный backup по умолчанию при работающих application services.
  Ожидается:
  успешный exit; полный набор артефактов; manifest TXT/JSON; оба checksum sidecar; `consistent_snapshot=true`; runtime остановлен перед backup и возвращён после backup.
  Статус источника: `нужно доснять из v1`.

- `BKP-002` Полный backup по умолчанию, когда application services уже были остановлены.
  Ожидается:
  успешный exit; полный набор артефактов; no unnecessary runtime start after backup; `app_services_were_running=false`.
  Статус источника: `нужно доснять из v1`.

- `BKP-003` Полный backup с `--no-stop`.
  Ожидается:
  успешный exit; полный набор артефактов; `consistent_snapshot=false`; runtime не останавливается и не возвращается; backup явно помечен как non-consistent snapshot.
  Статус источника: `нужно доснять из v1`.

### 2. Partial Paths

- `BKP-101` Files-only backup через `--skip-db`.
  Ожидается для `v2`:
  успешный exit; создаётся только `files_backup` и `files_checksum`; `db_backup` и `db_checksum` отсутствуют; manifest отсутствует; `details.skip_db=true`.
  Статус источника: `частично подтверждено v1`.
  Примечание:
  в `v1` files-only path уже наблюдается через CLI, но он всё ещё пишет manifest. Это legacy behavior и не переносится.

- `BKP-102` DB-only backup через `--skip-files`.
  Ожидается для `v2`:
  успешный exit; создаётся только `db_backup` и `db_checksum`; `files_backup` и `files_checksum` отсутствуют; manifest отсутствует; `details.skip_files=true`.
  Статус источника: `нужно доснять из v1`.

### 3. Fail-Closed Paths

- `BKP-201` Usage error: отсутствует или невалиден `--scope`.
  Ожидается:
  usage error до mutating path; backup не стартует; backup-артефакты не создаются.
  Статус источника: `подтверждено v1`.

- `BKP-202` Usage error: одновременно заданы `--skip-db` и `--skip-files`.
  Ожидается:
  usage error до mutating path; backup-артефакты не создаются.
  Статус источника: `подтверждено v1`.

- `BKP-203` Validation error: в env отсутствует `BACKUP_ROOT`.
  Ожидается:
  `backup_failed`; `ok=false`; mutating path не сообщает успех; complete backup-set не появляется.
  Статус источника: `подтверждено v1`.

- `BKP-204` Runtime prepare failure.
  Ожидается:
  backup не доходит до artifact finalization; blocked steps видимы в JSON semantics; complete backup-set не появляется.
  Статус источника: `нужно доснять из v1`.

- `BKP-205` Database dump failure.
  Ожидается:
  `files_backup`, `finalize` и `retention` не выполняются; mutating path fail-closed; если runtime был остановлен, выполняется явный runtime return или явная ошибка runtime return.
  Статус источника: `нужно доснять из v1`.

- `BKP-206` Files archive failure.
  Ожидается:
  `finalize` и `retention` не выполняются; success не сообщается; complete backup-set не появляется.
  Статус источника: `нужно доснять из v1`.

- `BKP-207` Finalize failure.
  Ожидается:
  manifest/checksum finalization fail-closed; retention не выполняется; complete backup-set не считается успешным.
  Статус источника: `нужно доснять из v1`.

- `BKP-208` Retention failure.
  Ожидается:
  backup не сообщает успех; operator видит explicit retention failure; нужно отдельно зафиксировать, какие уже созданные артефакты остаются на диске.
  Статус источника: `нужно доснять из v1`.

- `BKP-209` Runtime return failure после успешного создания backup-артефактов.
  Ожидается:
  overall result остаётся failed; backup не сообщает успех; operator видит explicit runtime return failure.
  Статус источника: `нужно доснять из v1`.

### 4. Disk Artifact Invariants

- `BKP-301` Полный backup создаёт canonical layout под `BACKUP_ROOT`.
  Ожидается:
  артефакты лежат под `db/`, `files/`, `manifests/`; имена используют один и тот же timestamp stamp.
  Статус источника: `частично подтверждено v1`.

- `BKP-302` Полный backup пишет согласованный JSON manifest.
  Ожидается:
  `version=1`; `scope`; `created_at`; basename-only artifact names; оба checksum; `db_backup_created=true`; `files_backup_created=true`; manifest валиден как complete backup-set contract.
  Статус источника: `частично подтверждено v1`.

- `BKP-303` Полный backup пишет согласованный text manifest.
  Ожидается:
  text manifest отражает complete backup-set; содержит `retention_days`, flags created/skipped, checksum filenames и size metadata для созданных артефактов.
  Статус источника: `нужно доснять из v1`.

- `BKP-304` Каждый созданный backup artifact имеет checksum sidecar.
  Ожидается:
  sidecar существует для каждого созданного artifact; checksum совпадает с содержимым файла.
  Статус источника: `частично подтверждено v1`.

- `BKP-305` После успешного backup не остаётся `*.tmp` хвостов.
  Ожидается:
  временные файлы не висят в backup root после success.
  Статус источника: `нужно доснять из v1`.

- `BKP-306` Partial backup в `v2` не создаёт manifest.
  Ожидается:
  для `--skip-db` и `--skip-files` отсутствуют `manifest_txt` и `manifest_json`; machine output отражает отсутствие manifest без двусмысленности.
  Статус источника: `legacy divergence`.

### 5. Retention Behavior

- `BKP-401` Retention удаляет старый backup-set целиком, а не отдельные файлы.
  Ожидается:
  удаляются все canonical paths старого set; не остаётся новых orphan states.
  Статус источника: `подтверждено v1`.

- `BKP-402` Retention сохраняет свежий backup-set целиком.
  Ожидается:
  свежий set не трогается частично.
  Статус источника: `подтверждено v1`.

- `BKP-403` Retention не может quietly corrupt success semantics.
  Ожидается:
  если retention ломается, overall backup не считается успешным.
  Статус источника: `нужно доснять из v1`.

### 6. Runtime Side Effects

- `BKP-501` Default full backup останавливает application services перед backup и возвращает их после success.
  Ожидается:
  runtime side effects согласованы с `consistent_snapshot=true`.
  Статус источника: `нужно доснять из v1`.

- `BKP-502` Если application services уже были остановлены, backup не поднимает их без необходимости.
  Ожидается:
  runtime после backup соответствует исходному requested state.
  Статус источника: `нужно доснять из v1`.

- `BKP-503` `--no-stop` оставляет runtime нетронутым.
  Ожидается:
  no stop/start side effects; backup помечен как non-consistent snapshot.
  Статус источника: `частично подтверждено v1`.

- `BKP-504` Backup preflight не требует writable runtime storage.
  Ожидается:
  backup допускается при read-only runtime storage, если writable backup root доступен.
  Статус источника: `частично подтверждено v1`.

- `BKP-505` Helper fallback для files archive.
  Ожидается:
  при отказе локального archive path backup всё ещё может завершиться успешно через explicit helper contract; создаётся читаемый files artifact; operator видит semantic warning о helper fallback.
  Статус источника: `нужно доснять из v1`.

## Предлагаемая структура хранения corpus

Первый нейтральный layout для `backup v2`:

```text
acceptance/
  v2/
    backup/
      ACCEPTANCE.md
      cases/
      fixtures/
        env/
        storage/
        runtime/
      golden/
        json/
        disk/
```

Назначение:

- `ACCEPTANCE.md`: человекочитаемый contract и scenario catalog
- `cases/`: machine-readable case definitions по сценарию
- `fixtures/env/`: входные env fixtures
- `fixtures/storage/`: входные файловые деревья
- `fixtures/runtime/`: declarative runtime-state fixtures
- `golden/json/`: normalized JSON outputs
- `golden/disk/`: expected disk-state snapshots

## Что уже можно брать из `v1` как black-box reference

- `internal/cli/backup_validation_test.go`
  usage error для `--scope` и `--skip-db --skip-files`

- `internal/cli/backup_execute_schema_test.go`
  files-only success path через CLI JSON и validation error при пустом `BACKUP_ROOT`

- `internal/cli/backup_execute_golden_test.go`
  normalized JSON golden для files-only `--skip-db --no-stop`

- `internal/app/internal/backupflow/retention_test.go`
  observable disk behavior retention по whole backup-set

- `internal/app/operation/operation_context_test.go`
  backup preflight для read-only runtime storage

## Что ещё нужно доснять из `v1` до cutover `backup`

- полный backup happy path с app services running
- полный backup happy path с app services already stopped
- полный backup с `--no-stop`
- db-only path через `--skip-files`
- command-level helper fallback success path
- runtime_prepare failure path
- db dump failure path
- files archive failure path
- finalize failure path
- retention failure path
- runtime_return failure path
- exact disk post-conditions после failed retention
- exact disk post-conditions после failed runtime return
- canonical full-backup text manifest snapshot как black-box artifact

## Открытые вопросы

- Должен ли initial `backup v2` corpus фиксировать exact order `items[*]`, или только обязательное множество `code/status`?
  Сейчас разумнее фиксировать порядок для full happy path и fail-closed path, но не навязывать порядок там, где `v2` сознательно расходится с `v1` по partial semantics.

- Нужно ли в initial corpus фиксировать exact human-readable text output для non-JSON режима?
  Сейчас ответ: нет. Для `v2` UI будет на русском, поэтому initial corpus должен сначала закрепить machine contract и disk/runtime post-conditions.

- Нужно ли сохранять text manifest для полного backup в `v2`?
  Пока в corpus предполагается, что да, потому что это наблюдаемая часть `v1` product behavior и она не конфликтует с `V2_SCOPE.md`.
