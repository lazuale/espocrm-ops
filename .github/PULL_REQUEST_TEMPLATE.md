## PR Gate

- [ ] This PR does not add a second code path for existing product behavior.
- [ ] This PR does not move shell execution outside `internal/runtime/docker.go`.
- [ ] This PR does not expand the shipped command surface.

If any statement above is false, the PR must be rejected or accompanied by a direct repo-contract update in `AGENTS.md`, `README.md`, and `CONTRIBUTING.md`.

## Verification

- [ ] `make ci`
