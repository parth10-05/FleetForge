# FleetForge Agent Instructions

## Scope
- Phase 02 is a pure-domain checkpoint only.
- Phase 03 builds durable PostgreSQL persistence only.
- Do not add HTTP business routes, Redfish calls, SQS/AWS, cloud deployment,
  Neon bootstrap, or firmware side effects unless a later phase explicitly requires it.

## Safety
- Keep transactions short. Never call network, storage, queue, or logging
  exporters while a transaction or database lock is held.
- Use strict Phase 02 parsers and invariants for persisted state.
  Unknown persisted values must fail closed.
- Preserve durable enrollment, admission, and operation-reservation semantics.
- Do not force-push, hard-reset, or delete user-owned untracked files.

## Workflow
- Read the active phase handover and implementation plan before coding.
- Complete cards in the documented order.
- Run each card's focused test before proceeding.
- Use descriptive, coherent commits—not tiny edit commits.
- Push to the active feature branch; do not merge a draft PR without explicit approval.

## Validation
- Do not use Docker locally.
- Run Docker-free Go tests, vet, domain property tests, and builds locally.
- PostgreSQL integration proof and `go test -race ./...` must be obtained in
  GitHub Actions Linux CI; do not claim them as local Windows results.