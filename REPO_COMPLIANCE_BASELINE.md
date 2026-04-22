# Repository Compliance Baseline

This file records the accepted architecture baseline for this repository.

It is the frozen result of a full audit against [REPO_COMPLIANCE_CHECKLIST.md](REPO_COMPLIANCE_CHECKLIST.md).
It is not a design note.
It is the current compliance verdict baseline.

If a future change intentionally moves the repository away from this baseline, the change must ship with:

- an updated formal audit
- an updated baseline file
- any required constitutional changes in [ARCHITECTURE.md](ARCHITECTURE.md) and [MICRO_MONOLITHS.md](MICRO_MONOLITHS.md)

---

## Audit Metadata

- Date: `2026-04-22`
- Commit / branch: working tree on `ab8f833edff69a48a84eb32e8112d0b4337a8a6d` on `main`
- Reviewer: `Codex`
- Scope: full repository audit against `REPO_COMPLIANCE_CHECKLIST.md` after documentation sharpen and enforcement wave under the frozen internal micro-monolith constitution
- Final verdict: `PASS`

---

## Section Results

1. Documentation Consistency: `PASS`
2. Repo-Wide Architectural Guard: `PASS`
3. Local Architectural Guards: `PASS`
4. Layer Ownership Audit: `PASS`
4A. Micro-Monolith Constitution Audit: `PASS`
5. Mutating Workflow Model: `PASS`
6. Mutating Workflow Status Canon: `PASS`
7. Error Ownership Audit: `PASS`
8. Ports and Adapter Boundaries: `PASS`
8A. Micro-Monolith Interaction Model Audit: `PASS`
9. Policy Ownership Audit: `PASS`
9A. Semantic Ownership Map Audit: `PASS`
10. Duplicate Workflow Assembly Audit: `PASS`
10A. Ops Pipeline Audit: `PASS`
11. Access Surface Audit: `PASS`
11A. Operational Access Class Audit: `PASS`
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

## Canonical Micro-Monolith Baseline

The approved internal micro-monolith list is frozen to:

1. `CLI Edge Monolith`
2. `Operation Lifecycle Monolith`
3. `Backup Execution Monolith`
4. `Backup Verification Monolith`
5. `Restore Execution Monolith`
6. `Migration Execution Monolith`
7. `Doctor Diagnostic Monolith`
8. `Runtime Adapter Monolith`
9. `Config / Env Loading Monolith`
10. `Filesystem Restore/Archive Monolith`
11. `Backup Store Integrity Monolith`
12. `Locking / Exclusive Access Monolith`
13. `Journal / Operation Trace Monolith`
14. `Result / Output Contract Monolith`
15. `Error / Failure Semantics Monolith`

Their contours, allowed caller sets, access classes, local pipelines, interaction model, and semantic ownership map are frozen in [MICRO_MONOLITHS.md](MICRO_MONOLITHS.md).

---

## Review Gate Baseline

A change must be rejected if it does any of the following:

- expands top-level `internal/app/*` production surface beyond the canonical boundary shape
- adds, removes, splits, or merges an approved micro-monolith without a constitutional update
- introduces a direct caller edge forbidden by `MICRO_MONOLITHS.md`
- moves privileged access to a weaker access class or introduces a hidden side channel
- reintroduces direct `internal/app -> internal/platform/*` imports
- introduces a second owner for an existing operational semantic

---

## Current Proof Split

Repo-wide machine-enforced surfaces:

- layer and command dependency boundaries
- canonical top-level app boundary surface
- canonical public `ErrorCode()` ownership
- explicit production process-env access surface
- explicit production shell/`exec.Command` surface
- approved top-level app and shared-kernel import edges

Owner-local machine-enforced surfaces:

- `internal/cli` command-runner and transport-bridge locality
- `internal/platform/docker` low-level exec and helper-shell ownership
- `internal/platform/config` env-loading discipline
- `internal/platform/backupstore` exported surface and error-code discipline
- `internal/platform/fs` exported surface, error-code discipline, and local shell-seam locality
- `internal/platform/locks` exported surface and no hidden env/shell control path
- `internal/platform/journalstore` exported surface and narrow journal-writer surface
- `internal/opsconfig` exported surface and no IO/env/shell drift

Review-enforced but binding surfaces:

- the full caller matrix beyond import-level and named shared-kernel edges
- the full global ops pipeline traversal
- the access-class table beyond syntactic seam ownership
- the semantic ownership map where ownership spans shared packages and bridge files

---

## Verification

- `go test ./...` passed
- `make ci` passed
