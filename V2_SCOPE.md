# Область V2

Этот документ задаёт контролируемый путь замены для `espops v2`.

`v1` остаётся:

- spec harness
- regression oracle
- emergency patch lane

В `v1` допускаются только:

- исправления P0 и P1
- security fixes
- сопровождение acceptance corpus
- сопровождение golden outputs
- проверки bug parity

`v2` — это новое execution core.
Оно обязано сохранить корректное наблюдаемое поведение `v1`.
Оно не обязано сохранять внутреннюю форму `v1`, его внутренние слои или doctrinal repository-style правила.

Если `v2` развивается в том же репозитории, он должен оставаться изолированным от implementation constraints `v1` до тех пор, пока конкретная команда не готова к cutover.

## 1. Product Surface

`v2` ограничен паритетом по этим пяти командам:

- `backup`
- `backup verify`
- `restore`
- `migrate`
- `doctor`

До достижения паритета по этим пяти командам в `v2` запрещены новые команды, новые режимы и любое расширение product surface.

## 2. Hard Invariants

`v2` обязан сохранять такие жёсткие инварианты:

- manifest существует только как complete backup-set contract
- partial backup представляется только direct artifacts, а не partial manifest
- checksum обязателен для каждого backup artifact
- `doctor` и execute-path используют один и тот же runtime contract и одни и те же правила валидации
- adapters не принимают product или policy decisions
- policy живёт выше runtime execution
- любой destructive path работает fail-closed
- `backup`, `verify`, `restore` и `migrate` сообщают об успехе только после явных проверок
- lock discipline остаётся явным и детерминированным

## 3. Non-Goals

`v2` явно не должен тянуть за собой:

- новую архитектурную конституцию
- repository-shape policing
- style-enforcing repository tests
- дополнительную абстракцию до появления второго реального use-case
- новый internal abstraction layer, пока его не требуют минимум два реальных сценария
- совместимость с внутренними package boundaries `v1`
- долгоживущую dual implementation
- convenience indirection, которое не уменьшает operational risk

## 4. Acceptance Source Of Truth

Источник истины для acceptance в `v2` берётся из наблюдаемого поведения `v1`:

- CLI scenarios
- disk artifacts
- manifest files
- checksum files
- error cases
- dry-run outputs там, где это важно
- stateful post-conditions на диске и runtime side effects

Для каждой переносимой команды acceptance tests пишутся раньше implementation.
Implementation следует acceptance corpus, а не наоборот.

## 5. Cutover Rule

Команда не переключается на `v2`, пока old-vs-new acceptance corpus не проходит для её command surface.

Cutover выполняется по командам, а не по всему репозиторию и не по архитектурным срезам.

## 6. Package Boundaries

Package boundaries в `v2` намеренно маленькие:

- `cmd/espops`: только CLI wiring
- `internal/app`: workflows и product policy
- `internal/runtime`: только Docker и runtime execution
- `internal/store`: backup artifact IO, manifest IO, checksum IO и verification
- `internal/model`: contracts, invariants и shared types

Никакой другой internal layer по умолчанию не предполагается.

## 7. Deletion Policy

После cutover конкретной команды:

- старая implementation удаляется
- долгоживущий dual-stack запрещён
- compatibility shims допускаются только как временные
- `v1` остаётся regression oracle, а не второй активной архитектурой

Начальный порядок миграции:

1. определить `backup` acceptance corpus
2. реализовать `backup` в `v2`
3. переключить `backup` только после достижения parity
4. повторить тот же цикл для `backup verify`, затем для `restore`, затем для `migrate`, затем для `doctor`
