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
- Commit / branch: working tree on `0b62c39d328ba1f50fc58015d99bde50ab35528b` on `main`
- Reviewer: `Codex`
- Scope: full repository audit against `REPO_COMPLIANCE_CHECKLIST.md`, targeted owner-local proof-promotion for explicit bridge seams, and the first physical split pass where mixed-package split triggers had already fired under the frozen internal micro-monolith constitution
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
- claims machine-enforcement without an honest syntactic anchor or uses guard theatre
- introduces unnamed bridge files or package-wide shared-ownership claims inside mixed packages
- leaves a promotion-eligible `review-enforced but binding` seam unpromoted after the criteria in `MICRO_MONOLITHS.md` are met
- leaves a split-triggered semantic slice inside a mixed package without a dedicated physical contour

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

- `internal/cli` command-runner locality and explicit error transport bridge definitions
- `internal/cli/errortransport` dedicated error-transport contour
- `internal/cli/journalbridge` dedicated journal-projection bridge contour
- `internal/app/operation` dedicated lifecycle contour
- `internal/app/operationtrace` dedicated journal/runtime trace contour
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

Interpretation baseline:

- `repo-wide machine-enforced` claims remain limited to rules with stable repo-wide syntactic anchors in `repository_test.go`.
- `owner-local machine-enforced` claims remain limited to one owner-local physical contour and must not simulate the caller matrix, access-class table, or semantic ownership map.
- Mixed-package colocation does not create a second owner; bridge files must stay explicitly nameable and owner-bounded.
- `review-enforced but binding` seams remain binding until the promotion criteria in `MICRO_MONOLITHS.md` are met.
- Physical split remains trigger-based rather than default; once the split triggers in `MICRO_MONOLITHS.md` fire, the slice must move to a dedicated physical contour.
- Fake machine-enforcement and guard theatre remain non-compliant.

---

## Verification

- `go test ./...` passed
- `make ci` passed
