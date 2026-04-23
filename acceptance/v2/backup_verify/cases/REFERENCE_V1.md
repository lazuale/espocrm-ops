# Black-Box Reference: `backup verify v1`

Этот файл фиксирует, как `v1` используется для `backup verify v2`.

`v1` является только:

- spec harness
- regression oracle
- emergency patch lane

`v1` не является шаблоном архитектуры для `v2`.

## Что сравнивается

- CLI usage behavior для существующей surface `backup verify`
- `--manifest`
- `--backup-root`
- JSON envelope на success и failure paths
- exit codes
- selected manifest при verify по backup root
- disk post-conditions
- checksum/archive/missing-artifact failures

## Что не переносится как invariant

- exact English UI strings
- internal package boundaries
- legacy partial-manifest semantics
- transport quirks root-level failure envelope
- отсутствие direct DB/files flags в текущем CLI как запрет на v2 core direct verification

## Reference bundles

Golden bundles лежат в:

- `acceptance/v2/backup_verify/golden/json/v1_BKV-*.json`
- `acceptance/v2/backup_verify/golden/disk/v1_BKV-*.json`

`v2` bundles для real CLI path после cutover лежат рядом:

- `acceptance/v2/backup_verify/golden/json/v2_BKV-*.json`
- `acceptance/v2/backup_verify/golden/disk/v2_BKV-*.json`

Если `v1` показывает legacy envelope shape, golden фиксирует это как reference fact.
В `v2` обязательными считаются machine contract и observable semantics, описанные в `ACCEPTANCE.md`.

После cutover `v1_BKV-*` bundles остаются oracle-only.
Текущий CI проверяет `v2_BKV-*` через реальный CLI path.

После контролируемого удаления старый command-specific `internal/app/backupverify` path удалён.
Оставшиеся legacy helpers вокруг manifest/checksum/store считаются retained только для `restore` / `migrate` или oracle/reference сценариев.
