# Micro-Monolith Constitution

This document is the binding micro-monolith constitution for `lazuale/espocrm-ops`.

It does not replace the layer constitution in [ARCHITECTURE.md](ARCHITECTURE.md).
It freezes the bounded internal operational units that exist inside the repository's one strict modular monolith.

The repository remains:

- one process boundary
- one application core
- one domain layer
- one adapters layer
- one privileged execution surface

It does not become a set of microservices.

Inside that one strict modular monolith, the only approved internal micro-monoliths are the units declared below.
The list is finite.
No implicit split, merge, or shadow owner is allowed.

## 1. Constitutional Scope

This constitution is authoritative for:

- the approved internal micro-monolith list
- the contour of each micro-monolith
- the allowed caller set for each micro-monolith
- the access rights for each micro-monolith
- the canonical local pipeline for each micro-monolith
- the global ops pipeline for the retained product
- the semantic ownership map

Any change to any of those surfaces is an architectural change.
It must ship with synchronized updates to:

- [ARCHITECTURE.md](ARCHITECTURE.md)
- [AGENTS.md](AGENTS.md)
- [CONTRIBUTING.md](CONTRIBUTING.md)
- [REPO_COMPLIANCE_CHECKLIST.md](REPO_COMPLIANCE_CHECKLIST.md)
- [REPO_COMPLIANCE_BASELINE.md](REPO_COMPLIANCE_BASELINE.md)

## 1A. Proof Classes And Interpretation

This constitution uses exactly three proof classes:

- `repo-wide machine-enforced`: a binding constitutional rule backed by `repository_test.go` with a stable repo-wide syntactic anchor
- `owner-local machine-enforced`: a binding constitutional rule backed by the owning package's `architecture_test.go` with a stable owner-local syntactic anchor inside one declared physical contour
- `review-enforced but binding`: a binding constitutional rule that remains mandatory in review and audit even when the repository does not yet have an honest cheap machine guard for it

No constitutional rule may remain unclassified, vague, or open-ended.

Interpretation rules:

- The interaction model records direct operational invocation edges and named shared-kernel entry edges only.
- Passive use of shared failure, result, or domain vocabulary inside a unit contour is audited through contours and ownership rules, not through the caller matrix.
- A contour entry must name a concrete package, file, or explicit file family.
- Shared package colocation does not create a second owner, but any bridge file inside a mixed package must be named explicitly.
- No rule may be called `machine-enforced` unless the guard has a concrete syntactic anchor whose failure directly proves the claimed constitutional violation.

## 1B. Proof Split Policy

A rule is `repo-wide machine-enforced` only when all of the following are true:

- the invariant is genuinely repository-wide rather than package-local
- the root guard can point to a stable repo-wide syntactic anchor such as a package boundary, import edge, exported surface, named file family, or literal vocabulary surface
- the check is cheap, deterministic, and honest about what it proves
- guard failure corresponds directly to a real constitutional violation
- the rule does not require semantic guessing, inferred callgraph reconstruction, or reviewer interpretation of operational meaning

A rule is `owner-local machine-enforced` only when all of the following are true:

- exactly one approved owner can enforce it
- the rule is local to one declared physical contour
- the local guard can point to a stable owner-local syntactic anchor
- the check proves local seam discipline, exported surface, env/shell seam locality, or explicitly named bridge discipline without claiming repo-wide meaning
- the guard does not simulate the full caller matrix, access-class table, or semantic ownership map

A rule remains `review-enforced but binding` when any of the following is true:

- no honest cheap machine guard exists yet
- the best available check would require semantic guessing
- the rule spans mixed packages, bridge files, multiple owners, or global pipeline traversal that syntax alone does not prove
- the rule is constitutionally mandatory even though proof currently lives in review and audit rather than tests

Forbidden proof claims:

- machine-enforcement without a real syntactic anchor
- a guard whose doctrinal claim is stronger than its proof surface
- semantic guessing encoded as guard logic
- brittle AST theatre used to declare ownership or caller legality
- pseudo-callgraph tests that do not prove real owner or allowed-caller facts

## 1C. Bridge Discipline Policy

Definitions:

- `semantic slice`: a bounded operational meaning owned by exactly one approved micro-monolith, even when its files currently live inside a shared Go package
- `physical contour`: a package, file, or explicit file family that can be named in this constitution and guarded honestly without semantic guessing
- `mixed package`: a Go package that contains files from more than one semantic slice under this constitution
- `bridge file`: an explicitly named file inside a mixed package that connects two declared slices without creating shared ownership

Rules:

- Semantic split may become binding before package split.
- Mixed package colocation does not create a second owner.
- Every non-bridge file inside a mixed package remains owned by exactly one semantic slice.
- Every bridge file must be named explicitly in the owning contour; anonymous shared helpers, catch-all file families, or package-wide bridge claims are forbidden.
- Every bridge file must remain owner-bounded: one canonical owner is responsible for its rule surface, callers, and drift budget even when the file touches another slice.
- Bridge files may carry translation, routing, or narrow shared-kernel seams only; they must not become a second policy owner or a miscellaneous staging area.
- A mixed package must keep bridge files few enough to name exhaustively and review as a finite list. If the seam needs wildcard ownership, anonymous helper families, or package-wide exceptions, the bridge layer is already overgrown.
- Mixed package colocation does not relax caller discipline, access discipline, or semantic ownership discipline.

## 1D. Promotion Criteria Policy

A `review-enforced but binding` rule must be promoted to machine enforcement when all of the following become true:

- a stable syntactic anchor exists
- an honest cheap guard exists
- the guard can prove the rule without semantic guessing
- the owner-local or repo-wide scope is explicit enough to choose the correct enforcement home

Promotion is mandatory in the same change, or at the latest the next intentional touch of that seam, if the criteria above are met and any of the following is true:

- the same seam has drifted more than once
- reviewers have had to restate the same rule in two or more distinct reviews
- a bridge file or finite explicit bridge-file set has become stable and uniquely nameable
- an owner-local physical contour now exists
- the rule has become routine mechanical review work rather than semantic judgment

Destination rules:

- promote to `owner-local machine-enforced` when the anchor lives inside one owner-local physical contour
- promote to `repo-wide machine-enforced` when the invariant is cross-repository and the root guard can prove it honestly
- do not promote when the candidate guard would overclaim, simulate semantics, or merely restate reviewer judgment in code

## 1E. Physical Split Trigger Policy

A semantic slice does not need a dedicated package by default.
Semantic split may remain inside a mixed package while Section `1C` remains honest.

A semantic slice must move to a dedicated physical contour once any of the following becomes true:

- the mixed package can no longer guard caller/callee locality or owner-local seam discipline without semantic guessing
- bridge files can no longer be listed exhaustively by explicit name
- local guards start simulating callgraph or cross-owner semantics instead of guarding local rules
- one slice acquires its own proof harness, lifecycle, or independently reviewed rule set
- one slice repeatedly drifts independently of its package neighbors
- one slice becomes independently privileged, independently reusable, or gains a distinct allowed-caller set
- package colocation now hides a real contour change that would otherwise require a constitutional update

When the existing package can no longer carry that contour honestly, the dedicated physical contour must be a separate package.
Physical split is trigger-based proof work, not symmetry cleanup and not a default requirement.

## 1F. Guard Theatre Prohibition Policy

`Guard theatre` is any claimed enforcement whose apparent rigor is stronger than its real proof.

Guard theatre includes:

- a guard that imitates proof but cannot point to a real violating surface
- a syntactic check that claims to prove semantic ownership or caller legality that the syntax does not actually prove
- an AST or regex test with a weak proxy signal and a strong doctrinal conclusion
- a pseudo-callgraph or pseudo-dataflow test that cannot establish the real owner, callee legality, or access class it claims to enforce
- duplicated repo-wide and owner-local guards that add no new proof value
- machine-enforced language in docs or review notes when the live proof is only local, partial, or review-only

Guard theatre is forbidden.

When the only honest proof is review, the rule must stay `review-enforced but binding` until Section `1D` promotion criteria are satisfied.
Better `review-enforced but binding` than fake `machine-enforced`.

## 2. Approved Micro-Monolith Map

The only approved internal micro-monoliths are:

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

No other internal operational unit may claim independent architectural status.

## 3. Micro-Monolith Passports

### 3.1 CLI Edge Monolith

#### A. Name
`CLI Edge Monolith`

#### B. Purpose
Own the process edge only:
parse flags, normalize operator input, enforce explicit confirmations, call one application boundary, and hand the completed result or failure to the output, journal, and error carriers.

#### C. Contour
Inside this unit:

- `cmd/espops/main.go` as process entrypoint and dependency wiring
- `internal/cli/root.go`
- `internal/cli/input.go`
- `internal/cli/options.go`
- `internal/cli/deps.go`
- command files under `internal/cli/` that define flags, validation, and boundary invocation:
  - `backup.go`
  - `backup_verify.go`
  - `restore.go`
  - `migrate.go`
  - `doctor.go`
- `internal/cli/execute.go` as root transport edge
- `internal/cli/runner.go` as the shared command-runner bridge into the journal, result, and error transport units

Allowed internal mechanisms:

- Cobra command wiring
- flag normalization
- project-relative path canonicalization
- explicit destructive confirmation gating
- dependency injection into application services
- command-runner routing into the journal, result, and error transport units

#### D. External Inputs
This unit may accept:

- process args and flags
- stdout/stderr handles
- CLI global options such as `--json` and `--journal-dir`
- explicit operator confirmations such as `--force` and `--confirm-prod`
- app/runtime/lock/journal dependencies injected at process start

#### E. External Outputs
This unit may emit:

- one normalized request into exactly one top-level app boundary
- one call into the journal pipeline for `backup`, `backup verify`, `restore`, or `migrate`
- one handoff into the result contract pipeline
- one handoff into the error transport pipeline
- usage failures for invalid edge input

#### F. Access Rights
Allowed:

- read CLI flags and args
- normalize filesystem paths from explicit flag values
- gate destructive commands on explicit edge confirmations
- choose JSON versus text rendering mode
- choose journal directory as an explicit edge input

Forbidden:

- load contour env files directly
- call Docker or Compose directly
- call filesystem, lock, or backup-store adapters directly
- own workflow assembly
- own domain policy
- own final public error classification
- invent hidden process-env control paths

#### G. Availability
Allowed callers:

- the process entrypoint only

Allowed callees:

- `Backup Execution Monolith`
- `Backup Verification Monolith`
- `Restore Execution Monolith`
- `Migration Execution Monolith`
- `Doctor Diagnostic Monolith`
- `Journal / Operation Trace Monolith`
- `Result / Output Contract Monolith`
- `Error / Failure Semantics Monolith`

Forbidden callers:

- application, domain, and platform units must not call back into CLI edge logic

#### H. Pipeline

1. Parse command line.
2. Normalize flags and paths.
3. Validate edge-only constraints.
4. Enforce explicit destructive confirmation.
5. Call one app boundary.
6. Hand result to journal/result/error pipelines.
7. Return exit status.

#### I. Invariants

- One command enters one top-level app boundary.
- CLI validation stays edge-owned.
- Destructive confirmation stays edge-owned.
- The edge never becomes a second workflow assembler.

#### J. Anti-Invariants

- No adapter-owned behavior hidden behind CLI helpers.
- No direct privileged execution from CLI into Docker, filesystem, locks, or backup-store code.
- No edge-owned policy duplication.

### 3.2 Operation Lifecycle Monolith

#### A. Name
`Operation Lifecycle Monolith`

#### B. Purpose
Own the shared mutating-operation preflight context:
resolve the contour env, derive project-local runtime paths, verify filesystem coherence, acquire the shared operation lock, acquire the contour maintenance lock, and release them in one canonical path.

#### C. Contour
Inside this unit:

- `internal/app/operation/`

Allowed internal mechanisms:

- operation context shaping
- preflight env resolution
- runtime path readiness verification
- shared operation lock acquisition
- contour maintenance lock acquisition
- explicit release sequencing

#### D. External Inputs
This unit may accept:

- `OperationContextRequest`
- project directory
- contour scope
- operation name
- env-file override
- optional log writer for lock acquisition

#### E. External Outputs
This unit may emit:

- `OperationContext`
- typed `domainfailure.Failure` causes for env, IO, or lock preflight failure

#### F. Access Rights
Allowed:

- call the env loader capability
- resolve project-local runtime paths
- inspect path readiness through the files capability
- acquire and release shared operation and maintenance locks

Forbidden:

- mutate Docker/Compose runtime
- mutate backup artifacts
- mutate restore state
- render result contracts
- write journal entries
- own final app/public error wrapping

#### G. Availability
Allowed callers:

- `Backup Execution Monolith`
- `Restore Execution Monolith`
- `Migration Execution Monolith`

Allowed callees:

- `Config / Env Loading Monolith`
- `Filesystem Restore/Archive Monolith`
- `Locking / Exclusive Access Monolith`

Forbidden callers:

- CLI edge
- doctor
- backup verification
- platform adapters

#### H. Pipeline

1. Accept operation request.
2. Load and validate contour env.
3. Derive compose project and backup root.
4. Verify runtime path readiness.
5. Acquire shared operation lock.
6. Acquire contour maintenance lock.
7. Return context.
8. Release locks explicitly on completion.

#### I. Invariants

- No mutating workflow reports readiness before this preflight succeeds.
- Shared and maintenance locks are acquired together or not at all.
- The returned context is the only canonical source for env file, compose project, and backup root inside mutating workflows.

#### J. Anti-Invariants

- No Docker access.
- No result rendering.
- No command-specific lock shortcuts.
- No hidden env fallback.

### 3.3 Backup Execution Monolith

#### A. Name
`Backup Execution Monolith`

#### B. Purpose
Own the canonical backup workflow and the shared backup snapshot kernel used by `backup` and the pre-restore emergency recovery point.

#### C. Contour
Inside this unit:

- `internal/app/backup/`
- `internal/app/internal/backupflow/`

Allowed internal mechanisms:

- backup request shaping from `OperationContext`
- backup artifact allocation
- runtime stop/start coordination
- DB dump creation
- files archive creation
- manifest and checksum finalization
- retention cleanup
- snapshot reuse by restore

#### D. External Inputs
This unit may accept:

- top-level `backup.Request`
- internal backup-flow `Request`
- `OperationContext`
- backup options such as `SkipDB`, `SkipFiles`, `NoStop`, and `Now`

#### E. External Outputs
This unit may emit:

- `backup.ExecuteInfo`
- backup artifacts under the resolved backup root
- manifest JSON and text files
- checksum sidecars
- workflow steps and warnings
- typed failures for backup boundary wrapping

#### F. Access Rights
Allowed:

- call the operation lifecycle preflight
- read env-derived backup settings from the operation context
- stop and start application services through the runtime capability
- dump the database through the runtime capability
- create local tar archives through the filesystem capability
- escalate to the Docker helper archive path when local archive creation fails, but only as an explicit in-pipeline branch that surfaces a warning
- write manifests and sidecars through the backup-store capability
- remove retention-expired backup sets

Forbidden:

- bypass operation preflight
- acquire restore DB/files locks
- choose restore or migration source selection policy
- render result or output contracts
- own final CLI confirmation rules
- hide fallback behavior from the reported workflow steps and warnings

#### G. Availability
Allowed callers:

- `CLI Edge Monolith` through `internal/app/backup`
- `Restore Execution Monolith` through the shared backup snapshot kernel

Allowed callees:

- `Operation Lifecycle Monolith`
- `Config / Env Loading Monolith`
- `Runtime Adapter Monolith`
- `Filesystem Restore/Archive Monolith`
- `Backup Store Integrity Monolith`
- `Error / Failure Semantics Monolith`

Forbidden callers:

- migration
- doctor
- backup verification
- platform adapters

#### H. Pipeline

1. Accept top-level backup request.
2. Run operation preflight.
3. Build canonical backup execution request.
4. Allocate backup artifacts.
5. Prepare runtime.
6. Dump database unless skipped.
7. Archive files unless skipped.
8. Finalize manifests and checksums.
9. Run retention cleanup.
10. Return runtime to the requested state.
11. Report explicit workflow result.

#### I. Invariants

- Backup success requires manifest finalization.
- Backup success requires explicit runtime return or an explicit skipped runtime-return step.
- The workflow uses the canonical mutating step vocabulary only.
- Snapshot backup inside restore uses this same kernel, not a parallel implementation.

#### J. Anti-Invariants

- No CLI-owned branching.
- No adapter-owned backup policy.
- No hidden backup naming or retention semantics outside this unit and the env/domain owners it calls.
- No success before explicit finalization and post-run runtime state accounting.

### 3.4 Backup Verification Monolith

#### A. Name
`Backup Verification Monolith`

#### B. Purpose
Own non-mutating backup-set verification from either an explicit manifest or a backup root that must resolve to one complete verified backup set.

#### C. Contour
Inside this unit:

- `internal/app/backupverify/`

Allowed internal mechanisms:

- explicit input exclusivity validation
- manifest resolution from backup root
- latest complete manifest selection
- backup-store verification report shaping

#### D. External Inputs
This unit may accept:

- manifest path
- backup root

#### E. External Outputs
This unit may emit:

- `backupverify.Report`
- verified manifest path
- resolved DB and files backup paths
- typed app failure for invalid verification input or invalid backup set

#### F. Access Rights
Allowed:

- list manifest candidates from a backup root
- verify a manifest-backed backup set through the backup-store capability

Forbidden:

- mutate Docker, filesystem, or locks
- load contour env
- render final output
- decide final transport semantics outside its boundary wrapper

#### G. Availability
Allowed callers:

- `CLI Edge Monolith`

Allowed callees:

- `Backup Store Integrity Monolith`
- `Error / Failure Semantics Monolith`

Forbidden callers:

- other app workflows
- adapters

#### H. Pipeline

1. Validate that exactly one source selector is present.
2. Resolve manifest path directly or from backup-root candidates.
3. Verify the resolved backup set.
4. Return the verified report.

#### I. Invariants

- Verification never succeeds without a complete verified backup set.
- Manifest-root selection and verification stay in one owner.

#### J. Anti-Invariants

- No runtime readiness semantics.
- No destructive operations.
- No second owner for backup-set verification policy.

### 3.5 Restore Execution Monolith

#### A. Name
`Restore Execution Monolith`

#### B. Purpose
Own the canonical destructive restore workflow, including source resolution, optional emergency recovery point, runtime preparation and return, DB restore, files restore, and dry-run planning.

#### C. Contour
Inside this unit:

- `internal/app/restore/`
- `internal/app/internal/restoreflow/`

Allowed internal mechanisms:

- restore source resolution
- dry-run plan shaping
- runtime inspection and preparation
- snapshot-before-restore backup invocation
- DB restore planning and execution
- files restore planning and execution
- permission reconciliation
- runtime return/start sequencing

#### D. External Inputs
This unit may accept:

- `restore.ExecuteRequest`
- `restoreflow.DBRequest`
- `restoreflow.FilesRequest`
- `OperationContext`
- manifest-backed or direct backup source paths

#### E. External Outputs
This unit may emit:

- `restore.ExecuteInfo`
- snapshot artifacts when snapshot is enabled
- DB and files restore plans
- workflow steps and warnings
- typed restore failures

#### F. Access Rights
Allowed:

- call the operation lifecycle preflight
- resolve restore sources through the backup-store capability
- invoke the backup execution kernel for the emergency recovery point
- inspect and prepare runtime state
- acquire restore DB/files locks
- reset and restore the target database
- unpack and replace the target files tree
- reconcile post-restore filesystem permissions
- return a dry-run plan instead of side effects when requested

Forbidden:

- bypass restore DB/files locks
- own migration source-selection policy
- own final public error mapping outside the restore boundary
- render result contracts
- infer hidden operator confirmation

#### G. Availability
Allowed callers:

- `CLI Edge Monolith` through `internal/app/restore`
- `Migration Execution Monolith` through the shared restore kernel

Allowed callees:

- `Operation Lifecycle Monolith`
- `Backup Execution Monolith`
- `Config / Env Loading Monolith`
- `Runtime Adapter Monolith`
- `Filesystem Restore/Archive Monolith`
- `Backup Store Integrity Monolith`
- `Locking / Exclusive Access Monolith`
- `Error / Failure Semantics Monolith`

Forbidden callers:

- doctor
- backup verification
- platform adapters

#### H. Pipeline

1. Accept restore request.
2. Run operation preflight.
3. Resolve and verify restore source.
4. Inspect runtime and shape the restore path.
5. Optionally create the emergency recovery point.
6. Prepare runtime.
7. Restore DB unless skipped.
8. Restore files unless skipped.
9. Reconcile permissions after files restore.
10. Return runtime to the requested post-restore state.
11. Report explicit result or dry-run plan result.

#### I. Invariants

- No destructive restore step executes before preflight and source verification succeed.
- DB restore and files restore each require their own restore lock.
- Dry-run stays within the same canonical pipeline and returns explicit planned/pending output instead of side effects.
- Snapshot, when enabled, uses the backup execution kernel instead of a second backup implementation.

#### J. Anti-Invariants

- No source-selection duplication in CLI.
- No second DB/files restore engine outside this unit.
- No hidden runtime switching.
- No success before explicit runtime-return or explicit `--no-start` accounting.

### 3.6 Migration Execution Monolith

#### A. Name
`Migration Execution Monolith`

#### B. Purpose
Own cross-contour migration of a verified backup set from a source contour into a target contour while reusing the restore kernel for destructive apply semantics.

#### C. Contour
Inside this unit:

- `internal/app/migrate/`

Allowed internal mechanisms:

- source contour preflight
- target contour preflight through the operation lifecycle
- source backup selection
- cross-contour compatibility checks
- target runtime preparation
- restore-kernel reuse for DB/files apply
- target runtime start

#### D. External Inputs
This unit may accept:

- `migrate.ExecuteRequest`
- source scope
- target scope
- explicit or automatic source backup selectors
- target runtime flags such as `SkipDB`, `SkipFiles`, and `NoStart`

#### E. External Outputs
This unit may emit:

- `migrate.ExecuteInfo`
- selected source artifacts
- workflow steps and warnings
- typed migration failures

#### F. Access Rights
Allowed:

- load source contour env directly
- call target contour operation preflight
- select source backup sets through the backup-store capability
- evaluate source/target compatibility through shared env policy
- prepare the target runtime
- call the restore execution kernel for DB and files application
- reconcile permissions after files migration
- start the target runtime unless explicitly disabled

Forbidden:

- mutate the source runtime
- implement a second DB/files restore engine
- bypass compatibility checks
- render output contracts
- own final public error mapping outside the migration boundary

#### G. Availability
Allowed callers:

- `CLI Edge Monolith`

Allowed callees:

- `Operation Lifecycle Monolith`
- `Restore Execution Monolith`
- `Config / Env Loading Monolith`
- `Runtime Adapter Monolith`
- `Backup Store Integrity Monolith`
- `Error / Failure Semantics Monolith`

Forbidden callers:

- other app workflows
- adapters

#### H. Pipeline

1. Load source contour env.
2. Run target contour operation preflight.
3. Resolve source backup selection.
4. Verify migration compatibility.
5. Prepare target runtime.
6. Restore DB into the target unless skipped.
7. Restore files into the target unless skipped.
8. Reconcile storage permissions when files changed.
9. Start the target runtime unless disabled.
10. Report explicit result.

#### I. Invariants

- Source and target contours differ.
- Compatibility passes before target mutation.
- DB/files apply semantics come from the restore execution kernel, not from a duplicate migration-local restore engine.

#### J. Anti-Invariants

- No source runtime mutation.
- No CLI-owned source-selection semantics.
- No second owner of migration compatibility meaning.

### 3.7 Doctor Diagnostic Monolith

#### A. Name
`Doctor Diagnostic Monolith`

#### B. Purpose
Own non-mutating runtime readiness diagnosis for the retained operational surface.

#### C. Contour
Inside this unit:

- `internal/app/doctor/`

Allowed internal mechanisms:

- shared compose-file and shared-lock checks
- Docker CLI/daemon/Compose readiness checks
- env load and env contract diagnosis
- runtime path readiness checks
- maintenance lock readiness checks
- running-service health diagnosis
- cross-scope isolation and compatibility checks

#### D. External Inputs
This unit may accept:

- `doctor.Request`
- target scope
- project directory
- compose file
- optional env-file override
- path-check mode

#### E. External Outputs
This unit may emit:

- `doctor.Report`
- readiness counts
- scope artifacts
- diagnostic checks with status, summary, details, and action

#### F. Access Rights
Allowed:

- load contour env files
- inspect compose configuration
- inspect Docker and Compose versions
- inspect running services and service health
- inspect path readiness
- inspect lock readiness
- compare dev/prod env compatibility when `--scope all`

Forbidden:

- acquire mutating locks
- mutate runtime or filesystem
- create journal entries
- map non-ready diagnostic results to final transport semantics inside helpers or adapters

#### G. Availability
Allowed callers:

- `CLI Edge Monolith`

Allowed callees:

- `Config / Env Loading Monolith`
- `Runtime Adapter Monolith`
- `Filesystem Restore/Archive Monolith`
- `Locking / Exclusive Access Monolith`

Forbidden callers:

- mutating workflows
- adapters

#### H. Pipeline

1. Validate target scope.
2. Check compose file.
3. Check shared operation lock readiness.
4. Check Docker CLI/daemon/Compose readiness.
5. Load requested contour env files.
6. Diagnose env contract and runtime paths.
7. Diagnose maintenance-lock and running-service health.
8. Run cross-scope checks when both contours are available.
9. Return the report.

#### I. Invariants

- Doctor remains read-only.
- Non-ready diagnostic meaning remains report-owned until the CLI edge maps it to transport failure.
- Cross-scope checks run only when both contours loaded successfully.

#### J. Anti-Invariants

- No mutating side effects.
- No app-owned exit-code mapping inside doctor helpers.
- No second readiness owner in adapters.

### 3.8 Runtime Adapter Monolith

#### A. Name
`Runtime Adapter Monolith`

#### B. Purpose
Own all Docker/Compose and helper-container access behind the runtime capability port.

#### C. Contour
Inside this unit:

- `internal/app/ports/runtimeport/`
- `internal/platform/appadapter/runtime.go`
- `internal/platform/docker/`

Allowed internal mechanisms:

- Compose target shaping
- Docker CLI execution
- runtime inspection
- runtime start/stop
- MySQL dump/restore execution
- helper-container archive creation
- helper-container storage-permission reconciliation

#### D. External Inputs
This unit may accept:

- `runtimeport.Target`
- service names
- container names and IDs
- DB credentials and archive paths
- helper image parameters

#### E. External Outputs
This unit may emit:

- runtime service lists
- service state and health metadata
- container IDs
- explicit runtime side effects
- typed adapter failures

#### F. Access Rights
Allowed:

- shell out to `docker`
- filter environment for Docker subprocesses
- call Docker Compose
- use helper-container execution for archive and permission tasks

Forbidden:

- own workflow policy
- own env resolution
- own manifest or backup-set policy
- own final public error codes
- use process stdout/stderr directly inside the adapter

#### G. Availability
Allowed callers:

- `Backup Execution Monolith`
- `Restore Execution Monolith`
- `Migration Execution Monolith`
- `Doctor Diagnostic Monolith`

Allowed callees:

- external Docker/Compose runtime only

Forbidden callers:

- CLI edge
- contract units
- journal unit

#### H. Pipeline

1. Accept runtime capability request.
2. Shape runtime-specific target.
3. Execute Docker/Compose or helper action.
4. Convert adapter-specific failures to typed capability failures.
5. Return explicit runtime result.

#### I. Invariants

- Low-level command execution remains in the Docker adapter.
- Helper shell seams stay in the storage-permissions helper owner.
- The adapter does not define `ErrorCode()` carriers.

#### J. Anti-Invariants

- No adapter-owned product policy.
- No CLI-only privileged shortcuts.
- No env-file discovery logic.

### 3.9 Config / Env Loading Monolith

#### A. Name
`Config / Env Loading Monolith`

#### B. Purpose
Own explicit contour env loading, project-relative path resolution, env-derived operational defaults, and DB password-source resolution.

#### C. Contour
Inside this unit:

- `internal/app/ports/envport/`
- `internal/platform/appadapter/env_loader.go`
- `internal/platform/config/`
- `internal/domain/env/`
- `internal/opsconfig/`

Allowed internal mechanisms:

- contour env-file path resolution
- env-file validation and parsing
- required key validation for operation loading
- project-relative path resolution
- password and password-file resolution
- shared env-derived policy such as backup retention, backup name prefix, and migration compatibility keys

#### D. External Inputs
This unit may accept:

- project directory
- contour scope
- env-file override
- DB password requests
- DB root-password requests

#### E. External Outputs
This unit may emit:

- `domainenv.OperationEnv`
- resolved project-local paths
- resolved DB password values
- typed validation or IO failures

#### F. Access Rights
Allowed:

- read explicitly selected env files
- read explicitly selected password files
- parse env assignments in Go
- resolve project-relative paths in Go

Forbidden:

- read ambient process env for product behavior
- shell out or use shell parsers
- own doctor's runtime contract diagnosis
- own workflow sequencing
- own final error codes

#### G. Availability
Allowed callers:

- `Operation Lifecycle Monolith`
- `Backup Execution Monolith`
- `Restore Execution Monolith`
- `Migration Execution Monolith`
- `Doctor Diagnostic Monolith`

Allowed callees:

- none outside its own contour

Forbidden callers:

- CLI edge for contour resolution
- runtime adapter
- result/error contract units

#### H. Pipeline

1. Resolve env-file path from project dir, contour, and optional override.
2. Validate the file contour and loading preconditions.
3. Parse env assignments.
4. Validate required values.
5. Construct `OperationEnv`.
6. Resolve project-local paths or DB passwords when requested.

#### I. Invariants

- Env meaning stays Go-owned.
- The contour env file is always explicit.
- Project-relative path resolution has one source of truth.

#### J. Anti-Invariants

- No process-env-only switches.
- No hidden path switching.
- No shell-owned config semantics.

### 3.10 Filesystem Restore/Archive Monolith

#### A. Name
`Filesystem Restore/Archive Monolith`

#### B. Purpose
Own local filesystem primitives for archive creation, archive unpacking, readiness inspection, staging, checksum calculation, and atomic tree replacement.

#### C. Contour
Inside this unit:

- `internal/app/ports/filesport/`
- `internal/platform/appadapter/files.go`
- `internal/platform/fs/`

Allowed internal mechanisms:

- tar.gz creation
- tar.gz unpacking
- checksum calculation
- directory readiness inspection
- writable-dir and free-space checks
- sibling staging
- prepared-tree discovery
- tree replacement

#### D. External Inputs
This unit may accept:

- source directory
- archive path
- directory readiness request
- stage target directory
- prepared tree parameters

#### E. External Outputs
This unit may emit:

- archives
- checksums
- readiness reports
- stage handles
- prepared tree roots
- replaced target trees
- typed filesystem/archive failures

#### F. Access Rights
Allowed:

- read and write local filesystem data
- create and remove staging directories
- inspect free space and writability

Forbidden:

- call Docker/Compose directly
- choose backup or restore workflow semantics
- choose manifest or source-selection policy
- own final public error codes

#### G. Availability
Allowed callers:

- `Operation Lifecycle Monolith`
- `Backup Execution Monolith`
- `Restore Execution Monolith`
- `Doctor Diagnostic Monolith`
- `Backup Store Integrity Monolith`

Allowed callees:

- host filesystem only

Forbidden callers:

- CLI edge
- result/error contract units

#### H. Pipeline

1. Accept explicit filesystem request.
2. Validate path preconditions.
3. Execute the filesystem primitive.
4. Return explicit result or typed failure.

#### I. Invariants

- Tree replacement stays stage-based and fail-closed.
- Exported filesystem surface stays intentional and finite.
- The adapter does not define `ErrorCode()` carriers.

#### J. Anti-Invariants

- No backup-set ownership.
- No runtime ownership.
- No workflow branching.

### 3.11 Backup Store Integrity Monolith

#### A. Name
`Backup Store Integrity Monolith`

#### B. Purpose
Own backup artifact naming, manifest schema, manifest/direct artifact verification, backup grouping, manifest loading, manifest writing, and checksum-sidecar writing.

#### C. Contour
Inside this unit:

- `internal/app/ports/backupstoreport/`
- `internal/platform/appadapter/backup_store.go`
- `internal/platform/backupstore/`
- `internal/domain/backup/`

Allowed internal mechanisms:

- canonical backup name parsing
- manifest validation
- manifest-backed artifact resolution
- direct backup verification
- backup candidate discovery
- backup grouping
- manifest persistence
- sidecar persistence

#### D. External Inputs
This unit may accept:

- manifest path
- direct DB backup path
- direct files backup path
- backup root
- manifest DTOs

#### E. External Outputs
This unit may emit:

- verified backup metadata
- manifest candidates
- grouped backup artifacts
- validated domain manifests
- written manifest files
- written checksum sidecars
- typed manifest and verification failures

#### F. Access Rights
Allowed:

- read manifest files
- read backup artifacts
- verify archive readability and checksum coherence
- write manifests and sidecars
- inspect backup-root grouping state

Forbidden:

- choose workflow branching
- choose source-selection policy
- mutate runtime
- acquire locks
- own final public error codes

#### G. Availability
Allowed callers:

- `Backup Execution Monolith`
- `Backup Verification Monolith`
- `Restore Execution Monolith`
- `Migration Execution Monolith`

Allowed callees:

- `Filesystem Restore/Archive Monolith`

Forbidden callers:

- CLI edge
- journal unit
- result/error contract units

#### H. Pipeline

1. Parse manifest or backup path input.
2. Validate canonical naming and coherence.
3. Verify checksums and archive readability or persist validated manifest data.
4. Return explicit verified metadata or typed failure.

#### I. Invariants

- Manifest verification and direct-backup verification stay under one owner.
- Manifest meaning stays anchored to `internal/domain/backup`.
- Exported backup-store surface stays intentional and finite.

#### J. Anti-Invariants

- No workflow ownership.
- No final transport error mapping.
- No lock or runtime ownership.

### 3.12 Locking / Exclusive Access Monolith

#### A. Name
`Locking / Exclusive Access Monolith`

#### B. Purpose
Own shared-operation locking, contour-maintenance locking, restore DB/files locking, and lock readiness diagnosis.

#### C. Contour
Inside this unit:

- `internal/app/ports/lockport/`
- `internal/platform/appadapter/locks.go`
- `internal/platform/locks/`

Allowed internal mechanisms:

- file-lock acquisition
- file-lock release
- lock readiness inspection
- stale-lock detection

#### D. External Inputs
This unit may accept:

- project directory
- backup root
- contour scope
- operation scope
- restore lock directory
- optional lock log writer

#### E. External Outputs
This unit may emit:

- `Releaser` handles
- readiness reports
- typed lock-acquisition and lock-readiness failures

#### F. Access Rights
Allowed:

- create lock files
- read lock metadata
- report stale or active lock state

Forbidden:

- decide when a workflow should lock
- mutate runtime
- mutate backup artifacts
- render output

#### G. Availability
Allowed callers:

- `Operation Lifecycle Monolith`
- `Restore Execution Monolith`
- `Doctor Diagnostic Monolith`

Allowed callees:

- host filesystem only

Forbidden callers:

- CLI edge
- backup verification
- result/error contract units

#### H. Pipeline

1. Accept lock or readiness request.
2. Resolve lock metadata path.
3. Inspect current state.
4. Acquire or report readiness explicitly.
5. Release explicitly when asked.

#### I. Invariants

- Mutating operation locking remains explicit.
- Restore DB/files sub-locks remain distinct from the shared operation lock.
- Readiness inspection and acquisition stay in the same owner.

#### J. Anti-Invariants

- No workflow policy ownership.
- No hidden lock fallback.
- No implicit lock bypass for command-specific branches.

### 3.13 Journal / Operation Trace Monolith

#### A. Name
`Journal / Operation Trace Monolith`

#### B. Purpose
Own operation IDs, timing, journal-entry schema, journal projection from result payloads, and filesystem persistence of journal entries.

#### C. Contour
Inside this unit:

- `internal/domain/journal/`
- `internal/app/operationtrace/`
- `internal/platform/journalstore/`
- `internal/cli/journalbridge/`

Allowed internal mechanisms:

- operation ID generation
- start/finish timestamp capture
- success/failure journal completion
- JSON-compatible journal payload projection
- journal file persistence

#### D. External Inputs
This unit may accept:

- command name
- result payload
- runtime clock and operation-ID generator
- configured journal writer

#### E. External Outputs
This unit may emit:

- persisted `domainjournal.Entry`
- completion timing data
- journal-shape warnings

#### F. Access Rights
Allowed:

- generate operation IDs
- capture timing
- serialize result payloads into journal-safe JSON shapes
- write JSON entries to the configured journal directory

Forbidden:

- decide workflow success or failure semantics
- choose result contract structure
- render final operator output
- call app workflows
- import `platform/journalstore` into `internal/app/operation`

#### G. Availability
Allowed callers:

- `CLI Edge Monolith` through the command runner only

Allowed callees:

- host filesystem through `internal/platform/journalstore`
- `Result / Output Contract Monolith` as the shape source for projected payloads

Forbidden callers:

- app workflow units directly
- platform adapters other than the journal store

#### H. Pipeline

1. Begin execution and allocate operation ID.
2. Capture start time.
3. Project result payload into journal-safe shapes.
4. Finish success or failure.
5. Persist one journal entry.
6. Return timing data and warnings.

#### I. Invariants

- Journal payloads stay JSON-compatible.
- One journal completion path exists for journaled commands.
- The app operation package never wires the concrete journal store directly.

#### J. Anti-Invariants

- No workflow branching.
- No final error classification.
- No adapter-owned journal semantics.

### 3.14 Result / Output Contract Monolith

#### A. Name
`Result / Output Contract Monolith`

#### B. Purpose
Own the external structured result DTOs and the text/JSON rendering of completed command results.

#### C. Contour
Inside this unit:

- `internal/contract/result/`
- command-local `*Result` and `render*Text` functions in:
  - `internal/cli/backup.go`
  - `internal/cli/backup_verify.go`
  - `internal/cli/restore.go`
  - `internal/cli/migrate.go`
  - `internal/cli/doctor.go`
- `internal/cli/execution_steps.go`

Allowed internal mechanisms:

- command-specific result DTO shaping
- item and section rendering
- JSON rendering
- text rendering
- warning rendering

#### D. External Inputs
This unit may accept:

- boundary `Info` or `Report` structs
- completion timing
- already-classified error DTOs
- JSON/text render mode

#### E. External Outputs
This unit may emit:

- `result.Result`
- text output
- JSON output

#### F. Access Rights
Allowed:

- shape command details/artifacts/items DTOs
- render to stdout/stderr writers
- render canonical workflow statuses

Forbidden:

- classify final public errors
- read env or runtime state
- call platform adapters
- decide workflow branching

#### G. Availability
Allowed callers:

- `CLI Edge Monolith`
- `Journal / Operation Trace Monolith` as a read-only payload-shape source
- `Error / Failure Semantics Monolith` when rendering transport failures

Allowed callees:

- writer outputs only

Forbidden callers:

- app workflows
- platform adapters

#### H. Pipeline

1. Accept finished app info/report or a classified error result.
2. Shape command DTOs.
3. Render text or JSON.

#### I. Invariants

- Result schemas remain in `internal/contract/result`.
- Mutating workflows expose only canonical workflow statuses.
- Presentation does not become a second owner of domain meaning.

#### J. Anti-Invariants

- No direct adapter access.
- No final error classification.
- No hidden result dialect.

### 3.15 Error / Failure Semantics Monolith

#### A. Name
`Error / Failure Semantics Monolith`

#### B. Purpose
Own failure kinds, boundary wrapping, final public error code mapping, CLI exit-code mapping, and error-result transport shaping.

#### C. Contour
Inside this unit:

- `internal/domain/failure/`
- `internal/contract/apperr/`
- `internal/contract/exitcode/`
- boundary error wrappers:
  - `internal/app/backup/errors.go`
  - `internal/app/backupverify/errors.go`
  - `internal/app/restore/errors.go`
  - `internal/app/migrate/compatibility.go`
- CLI error transport files:
  - `internal/cli/errors.go`
  - `internal/cli/result_error.go`
  - the root transport mapping in `internal/cli/execute.go`

Allowed internal mechanisms:

- domain failure classification
- boundary-to-app error wrapping
- error code normalization
- exit-code mapping
- result error shaping
- warning propagation on failures

#### D. External Inputs
This unit may accept:

- raw errors
- `domainfailure.Failure`
- local boundary failure types
- diagnostic non-ready signals from the CLI edge

#### E. External Outputs
This unit may emit:

- final app errors with `ErrorKind()` and `ErrorCode()`
- CLI `CodeError`
- CLI `ResultCodeError`
- classified `result.ErrorInfo`
- exit codes

#### F. Access Rights
Allowed:

- normalize local failure kinds and codes at app boundaries
- map final app errors to exit codes at the CLI edge
- attach failure warnings to transport errors

Forbidden:

- let helpers or adapters own final `ErrorCode()` carriers
- own workflow semantics
- perform side effects
- render final success payloads

#### G. Availability
Allowed callers:

- `Backup Execution Monolith`
- `Backup Verification Monolith`
- `Restore Execution Monolith`
- `Migration Execution Monolith`
- `CLI Edge Monolith`

Allowed callees:

- `Result / Output Contract Monolith` for error result rendering only

Forbidden callers:

- platform adapters as final error owners
- local helpers below the boundary level

#### H. Pipeline

1. Produce local domain/app failure.
2. Wrap it once at the top-level app boundary when required.
3. Map it once at the CLI edge into exit and public error semantics.
4. Render transport error result if needed.

#### I. Invariants

- The boundary is the final app error owner.
- CLI is the final transport error owner.
- Public `ErrorCode()` carriers stay limited to final app/transport wrappers.

#### J. Anti-Invariants

- No adapter-owned public codes.
- No helper-owned final wrapping.
- No second error-policy path for diagnostic readiness.

## 4. Interaction Model

This section records direct operational invocation edges and named shared-kernel entry edges only.
It does not try to enumerate passive use of shared failure/result/domain vocabulary that is already fixed by the owning contour.

### 4.1 Allowed Direct Caller -> Callee Edges

Only the following direct edges are approved:

- Process entrypoint -> `CLI Edge Monolith`
- `CLI Edge Monolith` -> `Backup Execution Monolith`
- `CLI Edge Monolith` -> `Backup Verification Monolith`
- `CLI Edge Monolith` -> `Restore Execution Monolith`
- `CLI Edge Monolith` -> `Migration Execution Monolith`
- `CLI Edge Monolith` -> `Doctor Diagnostic Monolith`
- `CLI Edge Monolith` -> `Journal / Operation Trace Monolith`
- `CLI Edge Monolith` -> `Result / Output Contract Monolith`
- `CLI Edge Monolith` -> `Error / Failure Semantics Monolith`
- `Operation Lifecycle Monolith` -> `Config / Env Loading Monolith`
- `Operation Lifecycle Monolith` -> `Filesystem Restore/Archive Monolith`
- `Operation Lifecycle Monolith` -> `Locking / Exclusive Access Monolith`
- `Backup Execution Monolith` -> `Operation Lifecycle Monolith`
- `Backup Execution Monolith` -> `Config / Env Loading Monolith`
- `Backup Execution Monolith` -> `Runtime Adapter Monolith`
- `Backup Execution Monolith` -> `Filesystem Restore/Archive Monolith`
- `Backup Execution Monolith` -> `Backup Store Integrity Monolith`
- `Backup Execution Monolith` -> `Error / Failure Semantics Monolith`
- `Backup Verification Monolith` -> `Backup Store Integrity Monolith`
- `Backup Verification Monolith` -> `Error / Failure Semantics Monolith`
- `Restore Execution Monolith` -> `Operation Lifecycle Monolith`
- `Restore Execution Monolith` -> `Backup Execution Monolith`
- `Restore Execution Monolith` -> `Config / Env Loading Monolith`
- `Restore Execution Monolith` -> `Runtime Adapter Monolith`
- `Restore Execution Monolith` -> `Filesystem Restore/Archive Monolith`
- `Restore Execution Monolith` -> `Backup Store Integrity Monolith`
- `Restore Execution Monolith` -> `Locking / Exclusive Access Monolith`
- `Restore Execution Monolith` -> `Error / Failure Semantics Monolith`
- `Migration Execution Monolith` -> `Operation Lifecycle Monolith`
- `Migration Execution Monolith` -> `Restore Execution Monolith`
- `Migration Execution Monolith` -> `Config / Env Loading Monolith`
- `Migration Execution Monolith` -> `Runtime Adapter Monolith`
- `Migration Execution Monolith` -> `Backup Store Integrity Monolith`
- `Migration Execution Monolith` -> `Error / Failure Semantics Monolith`
- `Doctor Diagnostic Monolith` -> `Config / Env Loading Monolith`
- `Doctor Diagnostic Monolith` -> `Runtime Adapter Monolith`
- `Doctor Diagnostic Monolith` -> `Filesystem Restore/Archive Monolith`
- `Doctor Diagnostic Monolith` -> `Locking / Exclusive Access Monolith`
- `Backup Store Integrity Monolith` -> `Filesystem Restore/Archive Monolith`
- `Journal / Operation Trace Monolith` -> `Result / Output Contract Monolith`
- `Error / Failure Semantics Monolith` -> `Result / Output Contract Monolith`

### 4.2 Forbidden Direct Edges

The following direct edges are forbidden:

- `CLI Edge Monolith` -> runtime, filesystem, backup-store, lock, or env adapters directly
- `Operation Lifecycle Monolith` -> `Runtime Adapter Monolith`
- `Operation Lifecycle Monolith` -> `Result / Output Contract Monolith`
- `Operation Lifecycle Monolith` -> `Journal / Operation Trace Monolith`
- `Backup Verification Monolith` -> runtime, filesystem, or locking units
- `Doctor Diagnostic Monolith` -> mutating workflow units
- `Migration Execution Monolith` -> source-runtime mutation paths
- platform adapter monoliths -> CLI edge or top-level workflow orchestration
- `Result / Output Contract Monolith` -> app workflows or adapters
- `Journal / Operation Trace Monolith` -> app workflow orchestration
- `Error / Failure Semantics Monolith` -> adapter execution

### 4.3 Special Routed Paths

The privileged mutating path is:

- `CLI Edge Monolith`
- one of `Backup Execution Monolith`, `Restore Execution Monolith`, or `Migration Execution Monolith`
- `Operation Lifecycle Monolith`
- the adapter monoliths required by that workflow
- `Error / Failure Semantics Monolith`
- `Journal / Operation Trace Monolith`
- `Result / Output Contract Monolith`

The diagnostic path is:

- `CLI Edge Monolith`
- `Doctor Diagnostic Monolith`
- read-only adapter monoliths
- `Result / Output Contract Monolith`
- `Error / Failure Semantics Monolith` only at CLI transport mapping

The backup verification path is:

- `CLI Edge Monolith`
- `Backup Verification Monolith`
- `Backup Store Integrity Monolith`
- `Journal / Operation Trace Monolith`
- `Result / Output Contract Monolith`
- `Error / Failure Semantics Monolith`

The final public error ownership path is:

- local failure inside the owning workflow or diagnostic unit
- boundary wrap in the owning top-level app boundary for `backup`, `backup verify`, `restore`, and `migrate` when a final app carrier is required
- direct report-to-transport mapping for non-ready `doctor` reports
- final exit/result mapping in `CLI Edge Monolith` through `Error / Failure Semantics Monolith`

The output contract path is:

- app info/report
- `Result / Output Contract Monolith`
- journal projection for journaled commands only
- final text or JSON rendering

## 5. Global Ops Pipeline

The global ops pipeline for the retained product is fixed to these phases:

Proof mode follows Section `1A`: some phases are machine-enforced, while others remain review-enforced but binding.

1. `Entry / edge`
   - Owner: `CLI Edge Monolith`
   - Parse command, flags, global options, and output mode.
2. `Input normalization`
   - Owner: `CLI Edge Monolith`
   - Canonicalize scope, project directory, compose path, env override, explicit artifact paths, and confirmation inputs.
3. `Request shaping`
   - Owners:
     - `CLI Edge Monolith` for transport request shaping
     - owning workflow monolith for operational request shaping
   - No second request shaper is allowed.
4. `Workflow execution`
   - Owner: selected app workflow or diagnostic monolith.
5. `Runtime interaction`
   - Owners: adapter monoliths called by the selected workflow.
6. `Verification / health-check`
   - Owner: selected workflow or diagnostic monolith.
   - Success must remain explicit.
7. `Result shaping`
   - Owner: `Result / Output Contract Monolith`.
8. `Error classification`
   - Owner:
     - top-level app boundary for final app wrapping when required
     - `CLI Edge Monolith` through `Error / Failure Semantics Monolith` for final transport mapping
9. `Journal`
   - Owner: `Journal / Operation Trace Monolith`
   - Applies to `backup`, `backup verify`, `restore`, and `migrate`
   - `backup verify` intentionally remains on the persisted journal path even though it is read-only
   - `doctor` intentionally stays outside persisted journaling and returns a diagnostic report only
10. `Output / render`
   - Owner: `Result / Output Contract Monolith`

### 5.1 Command Traversal

`backup` traverses:

- entry
- input normalization
- backup request shaping
- operation preflight
- backup execution
- finalize and retention verification
- error/result classification
- journal
- output

`backup verify` traverses:

- entry
- input normalization
- verification request shaping
- manifest resolution
- backup-store verification
- error/result classification
- journal
- output

`restore` traverses:

- entry
- input normalization
- restore request shaping
- operation preflight
- source resolution
- runtime planning and preparation
- optional emergency recovery point
- DB restore and/or files restore
- post-restore permission/runtime verification
- error/result classification
- journal
- output

`migrate` traverses:

- entry
- input normalization
- migration request shaping
- source env preflight
- target operation preflight
- source selection
- compatibility verification
- target runtime preparation
- target DB/files apply
- target runtime start verification
- error/result classification
- journal
- output

`doctor` traverses:

- entry
- input normalization
- diagnostic request shaping
- readiness diagnosis
- report shaping
- CLI transport mapping for non-ready reports
- output

### 5.2 Allowed Family-Specific Branches

The only approved branch categories are:

- skip branches explicitly represented in workflow steps such as `--skip-db`, `--skip-files`, `--no-stop`, `--no-start`, and `--no-snapshot`
- `restore --dry-run` planning through the same restore pipeline without destructive side effects
- `backup` local archive first, helper archive second, with an explicit surfaced warning if the helper path is used
- `doctor --scope all` adding cross-scope checks only after both contour env files load successfully
- `migrate` explicit versus automatic source selection, but still through one migration owner

Any branch that:

- bypasses `Operation Lifecycle Monolith`
- bypasses restore DB/files locks
- bypasses explicit post-check or health-check
- creates a second result, journal, or error path
- creates a second owner for the same semantic area

is a defect.

## 6. Operational Access Classes

| Micro-monolith | Access class | Destructive access | Explicit user confirmation may be honored here | Hidden side channels |
| --- | --- | --- | --- | --- |
| `CLI Edge Monolith` | transport edge | no | yes, and only here | forbidden |
| `Operation Lifecycle Monolith` | workflow coordination | lock acquisition only | no | forbidden |
| `Backup Execution Monolith` | privileged mutating workflow | yes | no | forbidden |
| `Backup Verification Monolith` | read-only verification | no | no | forbidden |
| `Restore Execution Monolith` | privileged destructive workflow | yes | no | forbidden |
| `Migration Execution Monolith` | privileged destructive workflow | yes, target contour only | no | forbidden |
| `Doctor Diagnostic Monolith` | read-only diagnostics | no | no | forbidden |
| `Runtime Adapter Monolith` | privileged adapter | yes, by workflow request only | no | forbidden |
| `Config / Env Loading Monolith` | explicit config access | env/password file reads only | no | forbidden |
| `Filesystem Restore/Archive Monolith` | privileged filesystem adapter | yes, by workflow request only | no | forbidden |
| `Backup Store Integrity Monolith` | backup-store adapter | manifest/sidecar writes only | no | forbidden |
| `Locking / Exclusive Access Monolith` | exclusive-access adapter | lock files only | no | forbidden |
| `Journal / Operation Trace Monolith` | journal adapter | journal-dir writes only | no | forbidden |
| `Result / Output Contract Monolith` | contract/output | no | no | forbidden |
| `Error / Failure Semantics Monolith` | contract/error | no | no | forbidden |

Additional access rules:

- Destructive operator confirmation is legal only in `CLI Edge Monolith`.
- Runtime, filesystem, backup-store, and lock adapters do not invent alternate control channels.
- `Doctor Diagnostic Monolith` and `Backup Verification Monolith` remain read-only even when they inspect privileged surfaces.

## 7. Semantic Ownership Map

| Semantic area | Canonical owner micro-monolith | Forbidden secondary owners |
| --- | --- | --- |
| CLI request validation and normalization | `CLI Edge Monolith` | app workflows, platform adapters |
| Shared mutating operation preflight | `Operation Lifecycle Monolith` | CLI edge, individual adapters |
| Backup execution meaning | `Backup Execution Monolith` | CLI edge, runtime adapter, restore-specific code outside the backup kernel |
| Backup verification meaning | `Backup Verification Monolith` | CLI edge, backup-store adapter |
| Restore execution meaning | `Restore Execution Monolith` | CLI edge, migration-local restore code, adapters |
| Migration source-selection and compatibility meaning | `Migration Execution Monolith` | CLI edge, restore code, adapters |
| Diagnostic readiness meaning | `Doctor Diagnostic Monolith` | CLI edge, runtime/config adapters |
| Docker/Compose runtime meaning | `Runtime Adapter Monolith` | CLI edge, workflow units as adapter owners |
| Contour env and path meaning | `Config / Env Loading Monolith` | CLI edge, runtime adapter, doctor contract checks beyond diagnosis |
| Local archive, stage, and tree-replacement meaning | `Filesystem Restore/Archive Monolith` | backup-store, CLI edge, workflow-local ad hoc filesystem code |
| Backup manifest and artifact integrity meaning | `Backup Store Integrity Monolith` | CLI edge, app workflows, filesystem adapter |
| Lock and exclusivity meaning | `Locking / Exclusive Access Monolith` | CLI edge, workflow-local ad hoc locks |
| Journal entry semantics | `Journal / Operation Trace Monolith` | CLI edge, app workflows, result renderer |
| Result DTO and output shape meaning | `Result / Output Contract Monolith` | app workflows, adapters |
| Final public error meaning | `Error / Failure Semantics Monolith` | helpers, adapters, local workflow internals below the boundary |

## 8. Review And Freeze Gate

A change must be rejected if it does any of the following without an explicit constitutional update:

- adds, removes, splits, or merges an approved micro-monolith
- changes a micro-monolith contour silently
- introduces a new direct caller edge not listed in this document
- moves privileged access to a weaker access class
- introduces a second semantic owner
- labels a rule `machine-enforced` without an honest syntactic anchor
- introduces an unnamed bridge file or package-wide shared-ownership claim inside a mixed package
- leaves a promotion-eligible `review-enforced but binding` seam unpromoted after Section `1D` is satisfied
- leaves a split-triggered semantic slice inside a mixed package after Section `1E` requires a dedicated physical contour
- creates a new non-canonical ops pipeline branch
- moves explicit user confirmation away from `CLI Edge Monolith`

This document is a freeze surface.
It must stay smaller, stricter, and more reliable than the code drift it prevents.
