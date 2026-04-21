# Repository Compliance Checklist

This checklist is the formal acceptance gate for architectural compliance.

It exists to verify that the repository matches the rules in `ARCHITECTURE.md`.
Passing tests alone is not sufficient.
The repository must also satisfy ownership, layering, workflow, policy, and access constraints.

Use only these result states for every section:

- `PASS`
- `PARTIAL`
- `FAIL`

Do not use vague conclusions such as:
- "mostly fine"
- "good enough"
- "acceptable for now"

If a section is not fully compliant, it is not `PASS`.

---

## 0. Audit Metadata

- Date:
- Commit / branch:
- Reviewer:
- Scope:
- Final verdict: `PASS` / `PARTIAL` / `FAIL`

---

## 1. Documentation Consistency

### Goal
Verify that architectural documents describe one consistent system.

### Check
Confirm that the following files agree on:
- strict modular monolith
- layer ownership
- CLI as edge-only
- `internal/app/` as workflow owner
- `internal/domain/` as policy owner
- `internal/platform/` as adapter owner
- canonical mutating workflow model
- canonical workflow status vocabulary
- prohibition on legacy compatibility shims
- prohibition on hidden architectural exceptions

### Files
- `ARCHITECTURE.md`
- `README.md`
- `CONTRIBUTING.md`

### Pass Criteria
- No contradictions between documents
- No stale references to removed layers or legacy architecture
- No document preserves old behavior "for compatibility"

### Findings
- Status:
- Notes:

---

## 2. Repo-Wide Architectural Guard

### Goal
Verify that the root architectural guard contains only repository-wide invariants.

### Check
Inspect `repository_test.go`.

It must contain only repo-wide rules such as:
- internal dependency boundaries
- backend prohibition on process-env reads
- canonical workflow vocabulary guard

It must not contain:
- package-specific style rules
- adapter-specific hygiene rules
- one-off local ownership rules
- broad stylistic doctrine unrelated to repo-wide architecture

### Pass Criteria
- Every rule in `repository_test.go` is cross-repository in scope
- No local/package-specific rule remains there

### Findings
- Status:
- Notes:

---

## 3. Local Architectural Guards

### Goal
Verify that local architectural rules live with their owners.

### Check
Inspect package-local `architecture_test.go` files.

Examples of correct locality:
- CLI package discipline in `internal/cli/...`
- docker adapter rules in `internal/platform/docker/...`
- `app/operation` ownership rules in `internal/app/operation/...`

### Pass Criteria
- Local rules live in their owner packages
- Local guards do not restate repo-wide rules unnecessarily
- There is no drift between local rules and `ARCHITECTURE.md`

### Findings
- Status:
- Notes:

---

## 4. Layer Ownership Audit

### Goal
Verify that each layer owns only its intended responsibilities.

### 4.1 CLI Layer
Check that `internal/cli/` only:
- parses flags
- validates input
- normalizes input
- calls one application boundary
- renders one structured result
- adapts result payloads for presentation/journal output

CLI must not:
- own product policy
- assemble workflow semantics in multiple ways
- own runtime semantics
- perform duplicate config derivation

#### Findings
- Status:
- Notes:

### 4.2 Application Layer
Check that `internal/app/` owns:
- workflow orchestration
- sequencing
- preflight coordination
- result shaping
- final error mapping

Application must not:
- import concrete infrastructure when a port should be used
- depend on CLI
- own presentation DTOs
- leak product policy into adapters

#### Findings
- Status:
- Notes:

### 4.3 Domain Layer
Check that `internal/domain/` owns:
- workflow vocabulary
- failure model
- compatibility policy
- runtime/env policy
- value semantics

Domain must:
- remain stdlib-only
- not depend on app, cli, platform, contract, or external libraries

#### Findings
- Status:
- Notes:

### 4.4 Platform Layer
Check that `internal/platform/` owns only:
- external access
- IO/integration logic
- adapter implementations

Platform must not own:
- workflow meaning
- compatibility semantics
- product policy
- domain vocabulary

#### Findings
- Status:
- Notes:

---

## 5. Mutating Workflow Model

### Goal
Verify that all mutating operations follow one canonical model.

### Scope
- backup
- restore
- migrate

### Required shape
- boundary is `Execute(req)`
- linear scenario
- structured info/result
- `Warnings`
- `Steps`
- `Counts()`
- `Ready()`

### Check
For each mutating operation, confirm:
- no imperative runner outlier remains
- no special-case workflow family remains
- no custom status dialect remains
- no boundary-specific exception remains

### Pass Criteria
All mutating workflows follow the same family of rules.

### Findings
- Status:
- Notes:

---

## 6. Mutating Workflow Status Canon

### Goal
Verify that mutating workflows use only one workflow status vocabulary.

### Canonical statuses
- `planned`
- `completed`
- `skipped`
- `blocked`
- `failed`

### Forbidden
- `would_run`
- `not_run`
- any parallel legacy status vocabulary

### Check
Inspect:
- domain vocabulary
- mutating app workflow code
- mutating CLI rendering
- mutating result contracts
- schema tests
- golden outputs
- root repo guard

Diagnostic/report boundaries may keep an explicit local diagnostic status vocabulary if they do not introduce a parallel mutating workflow dialect.

### Pass Criteria
- Canonical statuses only
- No compatibility translation remains
- No legacy mutating status literals remain in production mutating workflow code or output

### Findings
- Status:
- Notes:

---

## 7. Error Ownership Audit

### Goal
Verify that final external/app error mapping keeps one explicit owner.

### Check
Helpers may:
- return raw errors
- return local typed failures
- return domain/application failure classes

Helpers must not:
- perform final app/transport wrapping
- decide final public error code mapping

Application boundaries must own:
- final error classification
- final external/app wrapping

Diagnostic/report boundaries may return a structured report that the CLI maps to exit semantics when the report is non-ready, as long as helpers and adapters do not own that mapping.

### Pass Criteria
- No helper-level final error wrapping remains
- Boundary-owned wrapping is consistent across mutating workflows
- Diagnostic query readiness-to-exit mapping does not create a second helper/adapter policy owner

### Findings
- Status:
- Notes:

---

## 8. Ports and Adapter Boundaries

### Goal
Verify that application services depend on capability-oriented ports rather than concrete infrastructure.

### Check
Inspect `internal/app/ports/` and application modules.

Confirm:
- ports are grouped by capability
- there is no new god-hub port package
- app depends on ports, not concrete platform implementations
- platform implements ports
- ownership of interfaces is explicit

### Pass Criteria
- Ports are capability-scoped
- No application workflow imports concrete infrastructure where a port should exist
- Adapter implementations stay behind ports

### Findings
- Status:
- Notes:

---

## 9. Policy Ownership Audit

### Goal
Verify that each shared policy has exactly one owner.

### Check for duplication
Inspect ownership of:
- workflow vocabulary
- failure model
- runtime policy
- env policy
- compatibility rules
- readiness defaults
- operational service-role semantics

### Forbidden
- same policy defined in two layers
- policy duplicated across usecases
- policy living in infrastructure adapters
- policy assembled independently in multiple call paths

### Pass Criteria
Each shared policy can be pointed to in exactly one authoritative location.

### Findings
- Status:
- Notes:

---

## 10. Duplicate Workflow Assembly Audit

### Goal
Verify that the same operation semantics are not assembled in multiple places.

### Check
Look for duplication of:
- request construction
- env/config derivation
- path resolution semantics
- backup execution config shaping
- restore source shaping
- migration source shaping
- runtime preparation request shaping

### Forbidden
- CLI assembles one version while another usecase assembles another
- app layer duplicates config derivation already owned elsewhere
- same meaning built in parallel in two paths

### Pass Criteria
Every major operation has one owner for request/config/semantic assembly.

### Findings
- Status:
- Notes:

---

## 11. Access Surface Audit

### Goal
Verify that privileged behavior is explicit and centralized.

### Check
Confirm:
- one privileged execution path
- no command-specific privileged shortcuts
- no hidden env-driven control paths
- no duplicate runtime access paths
- no CLI-only privileged runtime behavior separate from app logic

Explicit CLI confirmation flags may gate destructive commands only as edge input validation; they must not bypass the canonical boundary or create a second runtime access path.

### Forbidden
- hidden operational side channels
- command-specific shortcuts around the canonical boundary
- duplicated journal/runtime wiring

### Pass Criteria
Privileged behavior is centralized, explicit, and consistent.

### Findings
- Status:
- Notes:

---

## 12. Testability Discipline Audit

### Goal
Verify that production code uses explicit testability mechanisms.

### Allowed
- request-level injection
- small local interfaces
- explicit test doubles

### Forbidden
- mutable package globals
- hidden test hooks
- ambient behavior
- test-only magical state

### Pass Criteria
Testability is explicit and local.

### Findings
- Status:
- Notes:

---

## 13. Anti-Legacy Audit

### Goal
Verify that no legacy-preserving architecture remains.

### Search for signals
Inspect production code and docs for:
- `legacy`
- `compat`
- `compatibility`
- `shim`
- `workaround`
- `temporary`
- `deprecated`
- "for compatibility"
- "old path"
- "keep old behavior"

### Pass Criteria
No legacy-preserving path remains in production architecture.
No comments are being used to justify architectural debt.

### Findings
- Status:
- Notes:

---

## 14. Result Contract Audit

### Goal
Verify that result and output contracts reflect the canonical architecture.

### Check
Inspect:
- `internal/contract/result/`
- CLI renderers
- schema tests
- golden files

Confirm:
- canonical workflow statuses only
- no parallel legacy DTO fields
- no stale output vocabulary
- no presentation-driven contamination of domain semantics

### Pass Criteria
Contracts reflect the canonical model directly.

### Findings
- Status:
- Notes:

---

## 15. Final Gate

### Mandatory commands
Run:
- `go test ./...`
- `make ci`

### Record
- `go test ./...`:
- `make ci`:

### Important
A green test run does not override architectural failure.
If any prior section is `FAIL`, the repository is not compliant even if tests pass.

---

## 16. Final Compliance Summary

### Repo-wide verdict
- `PASS` / `PARTIAL` / `FAIL`

### Blocking violations
- List blocking violations here

### Non-blocking concerns
- List non-blocking concerns here

### Required fixes before freeze
- List required fixes here

### Freeze recommendation
Choose one:
- `Freeze architecture`
- `Do not freeze; targeted fixes required`
- `Do not freeze; structural rewrite still required`

### Reviewer conclusion
Write a short final statement with no ambiguity.
