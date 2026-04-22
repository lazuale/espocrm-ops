# Repository Compliance Checklist

This checklist is the formal acceptance gate for architectural compliance.

It exists to verify that the repository matches the rules in `ARCHITECTURE.md` and `MICRO_MONOLITHS.md`.
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
- binding micro-monolith constitution
- layer ownership
- CLI as edge-only
- `internal/app/` as workflow owner
- `internal/domain/` as policy owner
- `internal/platform/` as adapter owner
- approved micro-monolith list
- micro-monolith interaction model
- micro-monolith access classes
- semantic ownership map
- global ops pipeline
- canonical mutating workflow model
- canonical workflow status vocabulary
- prohibition on legacy compatibility shims
- prohibition on hidden architectural exceptions
- proof split policy and proof-class definitions
- mixed-package and bridge-file discipline
- promotion criteria from `review-enforced but binding` to machine enforcement
- physical split trigger policy
- prohibition on guard theatre and false machine-enforcement

### Files
- `AGENTS.md`
- `ARCHITECTURE.md`
- `MICRO_MONOLITHS.md`
- `README.md`
- `CONTRIBUTING.md`
- `REPO_COMPLIANCE_CHECKLIST.md`
- `REPO_COMPLIANCE_BASELINE.md`

### Pass Criteria
- No contradictions between documents
- No stale references to removed layers or legacy architecture
- No document preserves old behavior "for compatibility"
- No contradiction between the layer constitution and the micro-monolith constitution
- Every constitutional rule can be identified as `repo-wide machine-enforced`, `owner-local machine-enforced`, or `review-enforced but binding`
- No document implies stronger proof than the repository actually carries
- No document treats mixed-package colocation as implicit shared ownership
- No document treats `review-enforced but binding` rules as optional or merely aspirational
- No document describes machine-enforcement without a named honest syntactic anchor

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
- pseudo-callgraph or weak-proxy AST tests that pretend to prove semantic ownership or caller legality
- review-only rules disguised as repo-wide proof

### Pass Criteria
- Every rule in `repository_test.go` is cross-repository in scope
- Every repo-wide guard has a stable repo-wide syntactic anchor
- No local/package-specific rule remains there
- No repo-wide guard claims stronger proof than it actually carries

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

When a local guard touches bridge discipline, it may guard only explicitly named owner-local bridge files or local seam facts.

### Pass Criteria
- Local rules live in their owner packages
- Every local guard has a stable owner-local syntactic anchor
- Local guards do not restate repo-wide rules unnecessarily
- No local guard simulates the full caller matrix, access-class table, or semantic ownership map
- There is no drift between local rules and `ARCHITECTURE.md` or `MICRO_MONOLITHS.md`

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
- routes one boundary result or failure through the canonical journal/result/error path
- renders one structured result or structured error output

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
- boundary info/report shaping
- final app-level error wrapping

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

## 4A. Micro-Monolith Constitution Audit

### Goal
Verify that the internal micro-monolith constitution is explicit, finite, and aligned with the current repository.

### Check
Inspect `MICRO_MONOLITHS.md`.

Confirm that:
- the approved micro-monolith list is finite
- every retained operational semantic is assigned to exactly one canonical micro-monolith owner
- every micro-monolith has an explicit passport covering:
  - name
  - purpose
  - contour
  - external inputs
  - external outputs
  - access rights
  - allowed caller set
  - local pipeline
  - invariants
  - anti-invariants
- the declared contours still match the actual package/file ownership in the repo
- shared bridge files inside mixed packages are named explicitly
- shared inner kernels are modeled as part of exactly one bounded micro-monolith rather than duplicated across commands
- the proof split policy uses only the three constitutional proof classes
- mixed packages, bridge files, semantic slices, and physical contours are defined without ambiguity
- promotion criteria and physical split triggers are explicit rather than implied

### Pass Criteria
- No missing retained-core micro-monolith
- No semantic overlap between micro-monolith owners
- No passport section left vague or open-ended
- No declared unit that does not map to current repository reality
- No mixed package implies shared ownership by colocation alone
- No bridge layer is anonymous, package-wide, or open-ended

### Findings
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
- implement public `ErrorCode()` carriers for adapter/local failures

Application boundaries must own:
- final error classification
- final app-level wrapping

The CLI edge must own:
- final transport exit-code mapping
- final public error-result transport mapping

Public `ErrorCode()` carriers must remain limited to final app/transport wrappers.

Diagnostic/report boundaries may return a structured report that the CLI maps to exit semantics when the report is non-ready, as long as helpers and adapters do not own that mapping.

### Pass Criteria
- No helper-level final error wrapping remains
- Boundary-owned wrapping is consistent across mutating workflows
- CLI-owned transport mapping is consistent across command families
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

## 8A. Micro-Monolith Interaction Model Audit

### Goal
Verify that cross-unit dependencies follow the approved micro-monolith interaction model.

### Check
Inspect `MICRO_MONOLITHS.md` and the production import/call structure.

Interpret the interaction model as the matrix of direct operational invocation edges and named shared-kernel entry edges.
Do not treat passive use of shared failure/result vocabulary as a caller-matrix violation unless the contour or ownership map is wrong.

Confirm that:
- caller -> callee edges stay inside the approved interaction model
- the privileged mutating path is explicit
- the diagnostic path is explicit
- the output contract path is explicit
- the final public error ownership path is explicit
- shared inner kernels are called only by their approved upstreams

### Forbidden
- CLI edge calling privileged adapters directly
- app units calling forbidden peer workflow units
- adapter units reaching back into orchestration or transport owners
- a second direct caller path around `Operation Lifecycle Monolith`

### Pass Criteria
- All direct architectural edges are approved
- No forbidden caller edge exists
- The interaction model remains one-way where declared one-way

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

## 9A. Semantic Ownership Map Audit

### Goal
Verify that the semantic ownership map in `MICRO_MONOLITHS.md` still matches the codebase.

### Check
Audit the declared canonical owners for:
- workflow meaning
- runtime meaning
- env/config meaning
- file/archive meaning
- backup-store meaning
- lock meaning
- journal meaning
- result contract meaning
- final public error meaning
- diagnostic readiness meaning
- bridge files and mixed packages where ownership spans shared colocation

### Pass Criteria
- Every semantic area still has one canonical owner
- No forbidden secondary owner appears in production code or docs
- The ownership map agrees with the layer model and ports/adapters boundaries
- No mixed package masquerades as dual ownership
- No bridge file has ambiguous owner or implicit privilege escalation

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

## 10A. Ops Pipeline Audit

### Goal
Verify that the repository still follows one explicit global ops pipeline with only approved family-specific branches.

### Check
Inspect `MICRO_MONOLITHS.md`, app workflows, CLI runner flow, and result/journal/error shaping.

Confirm the declared phases remain explicit:
- entry / edge
- input normalization
- request shaping
- workflow execution
- runtime interaction
- verification / health-check
- result shaping
- error classification
- journal
- output / render

Confirm command traversal for:
- `backup`
- `backup verify`
- `restore`
- `migrate`
- `doctor`

### Forbidden
- alternate hidden branch around the common pipeline
- command-specific success path without explicit verify/report/finalize semantics
- a new journal or error route outside the canonical pipeline

### Pass Criteria
- The common ops pipeline is still recognizable in code
- Family-specific branches remain explicit and owner-local
- No branch has become an architectural defect

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
- privileged access stays inside the approved micro-monolith access classes
- destructive operations stay inside their approved micro-monoliths
- explicit user confirmation stays edge-owned

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

## 11A. Operational Access Class Audit

### Goal
Verify that each micro-monolith still has the access class declared in `MICRO_MONOLITHS.md` and nothing stronger.

### Check
Inspect the declared access class table and compare it to current code.

Confirm that:
- transport-only units do not gain privileged adapter access
- read-only diagnostic or verification units do not mutate state
- privileged adapters do not become workflow owners
- journal and output units stay side-effect constrained to their declared surfaces
- each access-class claim is recorded as machine-enforced or review-enforced rather than assumed

### Pass Criteria
- Every micro-monolith still matches its declared access class
- No weaker unit has gained stronger hidden power
- No hidden side channel bypasses the declared access model

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
