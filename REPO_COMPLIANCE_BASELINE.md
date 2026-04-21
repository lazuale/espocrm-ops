# Repository Compliance Baseline

This file records the accepted architecture baseline for this repository.

It is the frozen result of a full audit against [REPO_COMPLIANCE_CHECKLIST.md](REPO_COMPLIANCE_CHECKLIST.md).
It is not a design note.
It is the current compliance verdict baseline.

If a future change intentionally moves the repository away from this baseline, the change must ship with:

- an updated formal audit
- an updated baseline file
- any required constitutional changes in [ARCHITECTURE.md](ARCHITECTURE.md)

---

## Audit Metadata

- Date: `2026-04-21`
- Commit / branch: working tree on `42e2140a0da25d6d6b15e64a88b778ad4e95f903` on `main`
- Reviewer: `Codex`
- Scope: full repository audit against `REPO_COMPLIANCE_CHECKLIST.md` after pre-freeze blocker closure
- Final verdict: `PASS`

---

## Section Results

1. Documentation Consistency: `PASS`
2. Repo-Wide Architectural Guard: `PASS`
3. Local Architectural Guards: `PASS`
4. Layer Ownership Audit: `PASS`
5. Mutating Workflow Model: `PASS`
6. Mutating Workflow Status Canon: `PASS`
7. Error Ownership Audit: `PASS`
8. Ports and Adapter Boundaries: `PASS`
9. Policy Ownership Audit: `PASS`
10. Duplicate Workflow Assembly Audit: `PASS`
11. Access Surface Audit: `PASS`
12. Testability Discipline Audit: `PASS`
13. Anti-Legacy Audit: `PASS`
14. Result Contract Audit: `PASS`
15. Final Gate: `PASS`
16. Final Compliance Summary: `PASS`

Current non-PASS items:

- none

---

## Canonical Boundary Baseline

Top-level application boundaries are frozen to this canon:

- `internal/app/backup`: `Execute(req)`
- `internal/app/backupverify`: `Diagnose(req)`
- `internal/app/restore`: `Execute(req)`
- `internal/app/migrate`: `Execute(req)`
- `internal/app/doctor`: `Diagnose(req)`

Top-level app boundary packages must not reintroduce exported production helper surfaces beyond:

- `Dependencies`
- `Service`
- `NewService`
- boundary request/report/info structs
- `Execute(req)` or `Diagnose(req)`
- `Counts()` / `Ready()` on boundary result types

Shared backup and restore execution primitives now live only under:

- `internal/app/internal/backupflow`
- `internal/app/internal/restoreflow`

These are internal owners, not public app boundaries.

---

## Review Gate Baseline

A change must be rejected if it does any of the following:

- expands top-level `internal/app/*` production surface beyond the canonical boundary shape
- reintroduces direct `internal/app -> internal/platform/*` imports
- introduces a second owner for an existing operational semantic

---

## Verification

- `go test ./...` passed
- `make ci` passed
