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

Ещё один зафиксированный black-box факт из `v1`:

- use-case failure на CLI JSON-уровне часто уходит через root-level transport envelope с `command == "espops"` и без `details/items/artifacts`

Это наблюдаемое поведение `v1`, но для `v2` оно считается `legacy transport quirk`.
Для fail-path acceptance в initial corpus жёстко фиксируются:

- `process_exit_code`
- `error.kind`
- `error.code`
- `error.exit_code`
- disk/runtime post-conditions

А exact failure envelope shape из `v1` не поднимается до обязательного invariants-контракта `v2`.

## Минимальный machine-readable contract для `backup`

Для `--json` initial corpus закрепляет:

- для success-path: `command == "backup"`
- `ok == true|false`
- для use-case failure: `error.code == "backup_failed"`
- `process_exit_code`
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
Для warnings в reference corpus фиксируется semantic слой, а не exact English wording.

## Каталог сценариев

Ниже перечислены сценарии, которые должны войти в acceptance corpus для `backup v2`.

Статус источника:

- `подтверждено v1`: уже есть пригодный black-box reference в `v1`
- `legacy divergence`: поведение `v1` зафиксировано, но перенос в `v2` запрещён `V2_SCOPE.md`

### 1. Happy Path

- `BKP-001` Полный backup по умолчанию при работающих application services.
  Ожидается:
  успешный exit; полный набор артефактов; manifest TXT/JSON; оба checksum sidecar; `consistent_snapshot=true`; runtime остановлен перед backup и возвращён после backup.
  Статус источника: `подтверждено v1`.

- `BKP-002` Полный backup по умолчанию, когда application services уже были остановлены.
  Ожидается:
  успешный exit; полный набор артефактов; no unnecessary runtime start after backup; `app_services_were_running=false`.
  Статус источника: `подтверждено v1`.

- `BKP-003` Полный backup с `--no-stop`.
  Ожидается:
  успешный exit; полный набор артефактов; `consistent_snapshot=false`; runtime не останавливается и не возвращается; backup явно помечен как non-consistent snapshot.
  Статус источника: `подтверждено v1`.

### 2. Partial Paths

- `BKP-101` Files-only backup через `--skip-db`.
  Ожидается для `v2`:
  успешный exit; создаётся только `files_backup` и `files_checksum`; `db_backup` и `db_checksum` отсутствуют; manifest отсутствует; `details.skip_db=true`.
  Статус источника: `подтверждено v1`.
  Примечание:
  в `v1` files-only path подтверждён отдельным reference bundle, но он всё ещё пишет manifest. Это legacy behavior и не переносится.

- `BKP-102` DB-only backup через `--skip-files`.
  Ожидается для `v2`:
  успешный exit; создаётся только `db_backup` и `db_checksum`; `files_backup` и `files_checksum` отсутствуют; manifest отсутствует; `details.skip_files=true`.
  Статус источника: `подтверждено v1`.
  Примечание:
  в `v1` db-only path тоже пишет manifest. Это зафиксировано как legacy behavior и не переносится в `v2`.

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
  backup не доходит до artifact finalization; complete backup-set не появляется.
  Для `v1` CLI transport подтверждён root-level failure envelope без step-list.
  Статус источника: `подтверждено v1`.

- `BKP-205` Database dump failure.
  Ожидается:
  `files_backup`, `finalize` и `retention` не выполняются; mutating path fail-closed; если runtime был остановлен, выполняется явный runtime return или явная ошибка runtime return.
  Статус источника: `подтверждено v1`.

- `BKP-206` Files archive failure.
  Ожидается:
  `finalize` и `retention` не выполняются; success не сообщается; complete backup-set не появляется.
  Статус источника: `подтверждено v1`.

- `BKP-207` Finalize failure.
  Ожидается:
  manifest/checksum finalization fail-closed; retention не выполняется; complete backup-set не считается успешным.
  Статус источника: `подтверждено v1`.

- `BKP-208` Retention failure.
  Ожидается:
  backup не сообщает успех; operator видит explicit retention failure; нужно отдельно зафиксировать, какие уже созданные артефакты остаются на диске.
  Статус источника: `подтверждено v1`.

- `BKP-209` Runtime return failure после успешного создания backup-артефактов.
  Ожидается:
  overall result остаётся failed; backup не сообщает успех; operator видит explicit runtime return failure.
  Статус источника: `подтверждено v1`.

### 4. Disk Artifact Invariants

- `BKP-301` Полный backup создаёт canonical layout под `BACKUP_ROOT`.
  Ожидается:
  артефакты лежат под `db/`, `files/`, `manifests/`; имена используют один и тот же timestamp stamp.
  Статус источника: `подтверждено v1`.

- `BKP-302` Полный backup пишет согласованный JSON manifest.
  Ожидается:
  `version=1`; `scope`; `created_at`; basename-only artifact names; оба checksum; `db_backup_created=true`; `files_backup_created=true`; manifest валиден как complete backup-set contract.
  Статус источника: `подтверждено v1`.

- `BKP-303` Полный backup пишет согласованный text manifest.
  Ожидается:
  text manifest отражает complete backup-set; содержит `retention_days`, flags created/skipped, checksum filenames и size metadata для созданных артефактов.
  Статус источника: `подтверждено v1`.

- `BKP-304` Каждый созданный backup artifact имеет checksum sidecar.
  Ожидается:
  sidecar существует для каждого созданного artifact; checksum совпадает с содержимым файла.
  Статус источника: `подтверждено v1`.

- `BKP-305` После успешного backup не остаётся `*.tmp` хвостов.
  Ожидается:
  временные файлы не висят в backup root после success.
  Статус источника: `подтверждено v1`.

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
  Статус источника: `подтверждено v1`.

### 6. Runtime Side Effects

- `BKP-501` Default full backup останавливает application services перед backup и возвращает их после success.
  Ожидается:
  runtime side effects согласованы с `consistent_snapshot=true`.
  Статус источника: `подтверждено v1`.

- `BKP-502` Если application services уже были остановлены, backup не поднимает их без необходимости.
  Ожидается:
  runtime после backup соответствует исходному requested state.
  Статус источника: `подтверждено v1`.

- `BKP-503` `--no-stop` оставляет runtime нетронутым.
  Ожидается:
  no stop/start side effects; backup помечен как non-consistent snapshot.
  Статус источника: `подтверждено v1`.

- `BKP-504` Backup preflight не требует writable runtime storage.
  Ожидается:
  backup допускается при read-only runtime storage, если writable backup root доступен.
  Статус источника: `подтверждено v1`.

- `BKP-505` Helper fallback для files archive.
  Ожидается:
  при отказе локального archive path backup всё ещё может завершиться успешно через explicit helper contract; создаётся читаемый files artifact; operator видит semantic warning о helper fallback.
  Статус источника: `подтверждено v1`.

## Контракт helper fallback

Для `backup v2` существует один явный контракт для fallback при архивировании файлов:

- `ESPO_HELPER_IMAGE` — единственный образ helper, который можно использовать для fallback
- образ должен быть доступен локальному Docker runtime; pull, candidate-list и implicit fallback image selection запрещены
- fallback выполняется только после отказа локального пути архивирования файлов
- если `ESPO_HELPER_IMAGE` пустой или архивирование через helper завершилось ошибкой, `backup` завершается fail-closed с `backup_failed`

Этот контракт не меняет семантику partial backup:

- полный backup после helper fallback всё равно обязан создать manifest полного backup-set
- partial backup после helper fallback остаётся direct artifact + checksum, без manifest

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

## Что уже зафиксировано в acceptance corpus

`v1` reference bundles теперь снимаются воспроизводимо через:

- [internal/cli/backup_acceptance_reference_test.go](/home/febinet/code/docker/internal/cli/backup_acceptance_reference_test.go)
- [acceptance/v2/backup/cases/REFERENCE_V1.md](/home/febinet/code/docker/acceptance/v2/backup/cases/REFERENCE_V1.md)

В corpus уже сохранены:

- success/fail JSON bundles в [acceptance/v2/backup/golden/json](/home/febinet/code/docker/acceptance/v2/backup/golden/json)
- disk/runtime snapshots в [acceptance/v2/backup/golden/disk](/home/febinet/code/docker/acceptance/v2/backup/golden/disk)

Coverage по этим bundles:

- `BKP-001`, `BKP-301`, `BKP-302`, `BKP-303`, `BKP-304`, `BKP-305`, `BKP-501`
- `BKP-002`, `BKP-502`
- `BKP-003`, `BKP-503`
- `BKP-101` и `BKP-102` с явной фиксацией legacy partial-manifest behavior в `v1`
- `BKP-204` ... `BKP-209`
- `BKP-403`, `BKP-504`, `BKP-505`

## Статус после cutover `backup`

Команда `backup` подключена к реальному CLI path через `backup v2`.
Старые `v1_*` bundles остаются только как regression oracle/spec harness.
`BKP-505` дополнительно закреплён `v2_*` bundles для helper fallback slice.

Не как gap, а как уже зафиксированные legacy/non-goal пункты остаются:

- `BKP-306`: partial manifest behavior из `v1` не переносится в `v2`
- root-level failure envelope `command == "espops"` в `v1` не считается обязательным `v2` invariant

## Открытые вопросы

- Нужно ли в initial corpus фиксировать exact human-readable text output для non-JSON режима?
  Сейчас ответ: нет. Для `v2` UI будет на русском, поэтому initial corpus должен сначала закрепить machine contract и disk/runtime post-conditions.

- Нужно ли `v2` сохранять root-level failure envelope quirk из `v1`, где command-level use-case failure рендерится как `command == "espops"`?
  Пока ответ: нет, это зафиксировано как legacy transport quirk. Но это решение должно быть явно подтверждено на этапе cutover `backup`.
