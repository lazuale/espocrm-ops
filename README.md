# espocrm-ops

Small local helper for one EspoCRM Docker Compose server.

## Backup

Creates a database dump and a storage archive.

```bash
./bin/espops backup --scope prod --project-dir /path/to/project
```

## Restore

Imports the database dump and replaces the storage directory.

```bash
./bin/espops restore --scope prod --project-dir /path/to/project --manifest /path/to/backups/prod/<timestamp>/manifest.json
```
