# `v1` Reference Bundles для `backup`

Этот файл связывает `BKP-*` с воспроизводимыми black-box reference bundles из `v1`.

Источник генерации:

- [internal/cli/backup_acceptance_reference_test.go](/home/febinet/code/docker/internal/cli/backup_acceptance_reference_test.go)

Обновление конкретного bundle:

```bash
UPDATE_ACCEPTANCE_BACKUP_REFERENCE=1 go test ./internal/cli -run 'TestAcceptanceReference_BackupV1_JSONAndDisk/BKP-001$' -count=1
```

Проверка без перегенерации:

```bash
go test ./internal/cli -run 'TestAcceptanceReference_BackupV1_JSONAndDisk/BKP-001$' -count=1
```

## Bundles

- `BKP-001`
  Подтверждает: `BKP-001`, `BKP-301`, `BKP-302`, `BKP-303`, `BKP-304`, `BKP-305`, `BKP-501`
  JSON: [v1_BKP-001.json](/home/febinet/code/docker/acceptance/v2/backup/golden/json/v1_BKP-001.json)
  Disk: [v1_BKP-001.json](/home/febinet/code/docker/acceptance/v2/backup/golden/disk/v1_BKP-001.json)

- `BKP-002`
  Подтверждает: `BKP-002`, `BKP-502`
  JSON: [v1_BKP-002.json](/home/febinet/code/docker/acceptance/v2/backup/golden/json/v1_BKP-002.json)
  Disk: [v1_BKP-002.json](/home/febinet/code/docker/acceptance/v2/backup/golden/disk/v1_BKP-002.json)

- `BKP-003`
  Подтверждает: `BKP-003`, `BKP-503`
  JSON: [v1_BKP-003.json](/home/febinet/code/docker/acceptance/v2/backup/golden/json/v1_BKP-003.json)
  Disk: [v1_BKP-003.json](/home/febinet/code/docker/acceptance/v2/backup/golden/disk/v1_BKP-003.json)

- `BKP-101`
  Подтверждает: `BKP-101`
  JSON: [v1_BKP-101.json](/home/febinet/code/docker/acceptance/v2/backup/golden/json/v1_BKP-101.json)
  Disk: [v1_BKP-101.json](/home/febinet/code/docker/acceptance/v2/backup/golden/disk/v1_BKP-101.json)
  Примечание: bundle фиксирует legacy partial-manifest behavior `v1`, который не переносится в `v2`.

- `BKP-102`
  Подтверждает: `BKP-102`
  JSON: [v1_BKP-102.json](/home/febinet/code/docker/acceptance/v2/backup/golden/json/v1_BKP-102.json)
  Disk: [v1_BKP-102.json](/home/febinet/code/docker/acceptance/v2/backup/golden/disk/v1_BKP-102.json)
  Примечание: bundle фиксирует legacy partial-manifest behavior `v1`, который не переносится в `v2`.

- `BKP-204`
  Подтверждает: `BKP-204`
  JSON: [v1_BKP-204.json](/home/febinet/code/docker/acceptance/v2/backup/golden/json/v1_BKP-204.json)
  Disk: [v1_BKP-204.json](/home/febinet/code/docker/acceptance/v2/backup/golden/disk/v1_BKP-204.json)

- `BKP-205`
  Подтверждает: `BKP-205`
  JSON: [v1_BKP-205.json](/home/febinet/code/docker/acceptance/v2/backup/golden/json/v1_BKP-205.json)
  Disk: [v1_BKP-205.json](/home/febinet/code/docker/acceptance/v2/backup/golden/disk/v1_BKP-205.json)

- `BKP-206`
  Подтверждает: `BKP-206`
  JSON: [v1_BKP-206.json](/home/febinet/code/docker/acceptance/v2/backup/golden/json/v1_BKP-206.json)
  Disk: [v1_BKP-206.json](/home/febinet/code/docker/acceptance/v2/backup/golden/disk/v1_BKP-206.json)

- `BKP-207`
  Подтверждает: `BKP-207`
  JSON: [v1_BKP-207.json](/home/febinet/code/docker/acceptance/v2/backup/golden/json/v1_BKP-207.json)
  Disk: [v1_BKP-207.json](/home/febinet/code/docker/acceptance/v2/backup/golden/disk/v1_BKP-207.json)

- `BKP-208`
  Подтверждает: `BKP-208`, `BKP-403`
  JSON: [v1_BKP-208.json](/home/febinet/code/docker/acceptance/v2/backup/golden/json/v1_BKP-208.json)
  Disk: [v1_BKP-208.json](/home/febinet/code/docker/acceptance/v2/backup/golden/disk/v1_BKP-208.json)

- `BKP-209`
  Подтверждает: `BKP-209`
  JSON: [v1_BKP-209.json](/home/febinet/code/docker/acceptance/v2/backup/golden/json/v1_BKP-209.json)
  Disk: [v1_BKP-209.json](/home/febinet/code/docker/acceptance/v2/backup/golden/disk/v1_BKP-209.json)

- `BKP-504`
  Подтверждает: `BKP-504`
  JSON: [v1_BKP-504.json](/home/febinet/code/docker/acceptance/v2/backup/golden/json/v1_BKP-504.json)
  Disk: [v1_BKP-504.json](/home/febinet/code/docker/acceptance/v2/backup/golden/disk/v1_BKP-504.json)

- `BKP-505`
  Подтверждает: `BKP-505`
  JSON: [v1_BKP-505.json](/home/febinet/code/docker/acceptance/v2/backup/golden/json/v1_BKP-505.json)
  Disk: [v1_BKP-505.json](/home/febinet/code/docker/acceptance/v2/backup/golden/disk/v1_BKP-505.json)
