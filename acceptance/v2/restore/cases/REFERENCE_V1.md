# Black-Box Reference Для `restore v1`

Этот файл фиксирует, как `v1` используется для `restore v2`.

`v1` является только:

- spec harness
- regression oracle
- emergency patch lane

`v1` не является шаблоном архитектуры для `v2`.

## Что Сравнивается

- CLI usage behavior текущей surface `restore`
- `--manifest`
- `--db-backup`
- `--files-backup`
- `--skip-db`
- `--skip-files`
- `--no-snapshot`
- `--snapshot-before-restore`
- `--no-stop`
- `--no-start`
- `--force`
- `--confirm-prod`
- JSON envelope на success и failure paths
- exit codes
- наблюдаемые side effects аварийного snapshot
- runtime stop/return/no-stop/no-start post-conditions
- disk post-conditions после files restore

## Что Не Переносится Как Инвариант

- exact English UI strings
- internal package boundaries
- legacy partial-manifest semantics
- transport quirks root-level failure envelope
- dry-run internals вне отдельного `restore v2` slice

## Reference Bundles

`v1` bundles для `restore` будут храниться рядом с `v2` internal bundles:

- `acceptance/v2/restore/golden/json/v1_RST-*.json`
- `acceptance/v2/restore/golden/disk/v1_RST-*.json`
- `acceptance/v2/restore/golden/runtime/v1_RST-*.json`

Первый internal slice фиксирует `v2_RST-*` bundles.
CLI cutover допускается только после отдельного parity review по этому corpus.
