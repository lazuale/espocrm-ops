## Architecture Gate

- [ ] This PR does not expand top-level `internal/app/*` production surface beyond the canonical boundary shape.
- [ ] This PR does not introduce direct `internal/app -> internal/platform/*` imports.
- [ ] This PR does not introduce a second owner for an existing operational semantic.

If any statement above is false, the PR must be rejected or explicitly accompanied by a constitutional architecture change and a new formal compliance baseline.

## Verification

- [ ] `make ci`
