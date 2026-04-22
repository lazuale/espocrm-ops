# Architecture Constitution

This repository is a strict operational system.
It is not a playground for mixed patterns, convenience abstractions, compatibility shims, or legacy-preserving glue.

The architecture in this repository is intentionally narrow, explicit, and enforceable.

---

## 1. Core Architectural Decision

This product is a **strict modular monolith**.

It is **not** split into microservices by command.
There is no separate backup service, restore service, migrate service, or doctor service.

This system has one shared privileged execution surface:

- one project directory
- one Docker/Compose runtime surface
- one env/config surface
- one backup root
- one operation lifecycle
- one journal lifecycle

Therefore the architecture is:

- one process boundary
- one application core
- one domain layer
- one adapters layer
- one privileged execution path

This repository has two binding constitutional layers:

- [ARCHITECTURE.md](ARCHITECTURE.md) fixes the layer model, boundary families, dependency rules, and repo-wide architectural invariants.
- [MICRO_MONOLITHS.md](MICRO_MONOLITHS.md) fixes the bounded internal micro-monolith units, their contours, their access classes, their allowed caller sets, their interaction model, and the global ops pipeline.

The approved internal micro-monolith list is finite.
Only the units declared in `MICRO_MONOLITHS.md` are allowed.
No implicit split, merge, or caller edge is permitted.

If remote execution is ever introduced, the only acceptable split is:

- control plane
- one executor/agent

Not one service per command.

---

## 2. Non-Negotiable Rules

### 2.1 No Legacy Compatibility Shims

Legacy compatibility is not a design goal.

The repository must not contain:

- compatibility wrappers for removed behavior
- legacy output vocabulary preserved “for consumers”
- duplicated old and new code paths
- comment-based exceptions explaining why bad structure remains
- temporary adapters that become permanent

If the old shape conflicts with the canonical architecture, the old shape must be removed.

### 2.2 No Hidden Exceptions

There are no undocumented special cases.

If the architecture needs two valid modes, both modes must be modeled explicitly and cleanly.
A comment is not an architectural boundary.

### 2.3 No Mixed Programming Styles

Production code must not mix different implementation schools for the same class of problem.

The repo must not contain:

- one mutating operation implemented as an imperative runner
- another mutating operation implemented as a structured workflow
- one package using mutable globals
- another package using explicit dependency injection
- one boundary returning raw strings
- another boundary returning structured workflow results

One canonical style only.

### 2.4 No Adapter-Owned Product Policy

Infrastructure/adapters must not own:

- workflow meaning
- operational service-role semantics
- readiness policy
- compatibility contracts
- failure semantics
- domain vocabulary

Adapters perform external access only.

### 2.5 No Duplicated Workflow Assembly

The same semantics must not be assembled in multiple places.

Forbidden:

- CLI building a request one way while another module builds the same meaning differently
- repeated env/config derivation across call sites
- duplicated operational request shaping
- duplicated runtime policy

One owner only.

### 2.6 No Convenience Architecture

This repository must not grow:

- generic orchestration engines
- framework packages
- service locators
- magic registries
- fake abstraction layers
- “base command” hierarchies
- indirection with no hard ownership benefit

Explicit code is preferred over clever architecture.

---

## 3. Layer Model

Only the following layer model is valid.

### 3.1 `cmd/`
Program entrypoint only.

Responsibilities:
- construct the CLI app
- wire runtime and writer factories
- start the root command

Must not own:
- business logic
- workflow branching
- operational semantics

### 3.2 `internal/cli/`
CLI edge only.

Responsibilities:
- parse flags
- validate flags
- normalize input
- call one application boundary
- route the boundary result or failure through the canonical journal/result/error transport path
- render one structured result or structured error output

Must not own:
- domain policy
- operational policy
- workflow assembly
- duplicate config derivation
- hidden environment behavior

### 3.3 `internal/app/`
Application layer.

Responsibilities:
- usecase boundaries
- workflow orchestration
- preflight coordination
- execution sequencing
- boundary info/report shaping
- final app-level error wrapping
- operation lifecycle coordination

Application owns the workflows.

### 3.4 `internal/domain/`
Domain layer.

Responsibilities:
- invariants
- shared workflow vocabulary
- compatibility policy
- failure classification
- runtime policy
- env policy
- value objects where justified

Domain must remain independent from infrastructure and CLI.

### 3.5 `internal/platform/`
Infrastructure adapters.

Responsibilities:
- Docker/Compose interaction
- filesystem operations
- env file IO if treated as infrastructure
- journal persistence
- locks
- external system access

Platform does not own product meaning.

### 3.6 `internal/app/ports/`
Capability-oriented application ports.

Responsibilities:
- explicit dependency boundaries between application workflows and infrastructure adapters

Ports are grouped by capability, not by one god-interface and not by one god-package.

### 3.7 `internal/contract/`
External output and contract types.

Responsibilities:
- output schema
- presentation DTOs
- exit codes
- transport/app error contract types where needed

Contract types must not become the domain model.

### 3.8 `internal/opsconfig/`
Shared Go-owned operational semantics.

Responsibilities:
- path/env-derived operational semantics that must remain in one Go-owned source of truth

This package exists to prevent duplication of operational meaning.

---

## 4. Allowed Boundary Shapes

There are only two approved application boundary families.

### 4.1 Mutating workflows

Mutating workflows must expose:

- `Execute(req)`

Examples:
- backup
- restore
- migrate

Mutating workflows must:
- run a linear scenario
- return structured info
- expose warnings
- expose steps
- expose counts
- expose ready/final-state evaluation

### 4.2 Diagnostic queries

Diagnostic/report-oriented modules may expose a diagnostic boundary such as:

- `Diagnose(req)`

This exists because diagnostic queries are not mutating workflows.
It is not a legacy exception.
It is a separate canonical family.
Diagnostic/report boundaries may keep an explicit diagnostic result vocabulary when that vocabulary stays local to the diagnostic family and does not create a parallel mutating workflow dialect.

No other boundary families are allowed without explicit architectural justification.

---

## 5. Canonical Mutating Workflow Model

All mutating operations must follow one model.

### 5.1 Request
A structured request object enters the application boundary.

### 5.2 Execution
The application service runs a linear scenario:
- resolve
- prepare
- execute
- finalize
- report

### 5.3 Result
The boundary returns structured info with:
- `Warnings`
- `Steps`
- `Counts()`
- `Ready()`

### 5.4 Step Vocabulary
The canonical mutating workflow vocabulary is:

- `planned`
- `completed`
- `skipped`
- `blocked`
- `failed`

No other workflow status literals are allowed in mutating workflow production code, mutating result contracts, or mutating output.

Forbidden:
- `would_run`
- `not_run`
- any parallel legacy vocabulary

Diagnostic/report boundaries may use an explicit diagnostic status vocabulary if that vocabulary remains local to the diagnostic family and does not redefine mutating workflow semantics.

---

## 6. Error Ownership Rule

Error ownership is strict.

### 6.1 Helpers may return:
- raw errors
- local typed failures
- domain/application failure classes

### 6.2 Helpers must not:
- apply final app/transport wrappers
- own final public error category mapping
- decide transport contract behavior
- implement public `ErrorCode()` carriers for adapter/local failures

### 6.3 Final owners are split but singular:
- the top-level application boundary owns final app-level wrapping and classification when a workflow must convert local failures into a final app carrier
- the CLI edge owns final transport exit-code and public error-result mapping

Public `ErrorCode()` carriers belong only to final app/transport wrappers.
Adapter-local and helper-local typed causes may stay typed, but they must not present themselves as final public error-code owners.

The boundary is the only place where final app-level error semantics may be decided.
The CLI edge is the only place where final transport exit/output error semantics may be decided.

For diagnostic/report boundaries, the boundary owns the report model and readiness semantics. The CLI edge may map a completed but non-ready diagnostic report to exit/error transport semantics when it does not introduce a second diagnostic policy owner.

---

## 7. Dependency Rules

### 7.1 CLI
CLI may depend on:
- application boundaries
- contract/output types
- presentation adapters

CLI must not depend on:
- domain policy packages directly for workflow meaning
- infrastructure adapters directly except explicit edge wiring/adapters

### 7.2 Application
Application depends on:
- domain
- capability ports
- minimal contract/error boundary packages only where required

Application must not depend on:
- concrete infrastructure implementations
- CLI packages

### 7.3 Domain
Domain depends on:
- stdlib only

Domain must not depend on:
- app
- cli
- platform
- contract
- external libraries

### 7.4 Platform
Platform implements application ports.

Platform must not own product policy.

---

## 8. Testability Rules

Testability must use explicit mechanisms.

Allowed:
- request-level injection
- small local interfaces
- explicit test doubles

Forbidden:
- mutable package-level hooks
- hidden test-time globals
- implicit ambient behavior

If something needs injection, model it explicitly.

---

## 9. Access Model

This repository is security-sensitive because it operates on stateful runtime surfaces.

Therefore:

- there must be one privileged execution path
- there must be no command-specific privileged shortcuts
- there must be no duplicate access surfaces
- there must be no hidden operational side channels
- there must be no CLI-only privileged runtime behavior separate from application logic

Explicit destructive confirmation flags may be enforced at the CLI edge as input validation when they only gate entry to the canonical application boundary and do not create an alternate runtime or recovery path.

All privileged runtime behavior must be explicit and centrally owned.
Operational access classes and their allowed privileged surfaces are frozen in `MICRO_MONOLITHS.md`.

---

## 10. What Counts as an Architectural Defect

Any of the following is a defect and must be removed:

- legacy compatibility mapping
- duplicated request assembly
- duplicated operational policy
- adapter-owned product rules
- mixed mutating workflow models
- mixed mutating workflow status vocabularies
- helper-owned final error wrapping
- package-global mutable hooks
- command-specific privilege paths
- comments used to justify architectural asymmetry
- convenience-driven exceptions

---

## 11. Canon Over Compatibility

When canonical architecture conflicts with old behavior, canonical architecture wins.

This repository does not preserve legacy shapes “just in case”.
This repository does not keep old output dialects “for compatibility”.
This repository does not support parallel old/new architectural paths.

The correct fix is to remove the old path and keep one canonical path.

---

## 12. Review Gate

Proof strength, mixed-package discipline, promotion criteria, and physical split triggers follow Sections `1A` through `1F` of [MICRO_MONOLITHS.md](MICRO_MONOLITHS.md).
Review must not claim stronger proof than the repository honestly carries.

A change must be rejected if it introduces any of the following:

- a new architectural exception
- a new micro-monolith, a silent micro-monolith split/merge, or a new direct caller edge outside `MICRO_MONOLITHS.md`
- a machine-enforced claim without an honest syntactic anchor
- an unnamed bridge file or package-wide shared-ownership claim inside a mixed package
- a promotion-eligible `review-enforced but binding` seam left unpromoted after the criteria in `MICRO_MONOLITHS.md` are met
- a split-triggered semantic slice left in a mixed package after `MICRO_MONOLITHS.md` requires a dedicated physical contour
- a new legacy shim
- a second owner for an existing policy
- a second way to assemble the same workflow semantics
- a direct application dependency on concrete infrastructure
- a new workflow vocabulary literal
- a mutable package-global behavior hook
- a hidden environment-driven control path
- a broad generic abstraction without a hard ownership payoff

---

## 13. Freeze Rule

The architecture should change only for one of these reasons:

- ownership is wrong
- policy lives in the wrong layer
- workflow duplication exists
- access boundaries are unclear
- canonical rules are violated

Architecture must not churn for aesthetics.
Architecture must not churn for symmetry alone.
Architecture must not churn for “cleaner patterns” without a concrete ownership problem.
Mixed-package colocation is acceptable only while it still carries honest contours, explicit bridge files, and truthful proof classification.
Physical split is trigger-driven proof work, not default cleanup.
If a rule is still only honestly `review-enforced but binding`, it remains binding; fabricating machine enforcement is architectural drift, not progress.

The goal is not novelty.
The goal is one explicit, enforceable, maintainable architecture.
