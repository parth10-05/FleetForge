# V3-P00 — Release Measurement Contract

## Goal

Freeze the evidence contract before measuring or tuning. Every v3 claim needs a named workload, safety oracle, environment, cost limit, and reproducible artifact path.

## Tasks

- [ ] **V3-P00-T01** Freeze the versioned scenario schema for L0–L6: targets, sites/racks, policy, concurrency, faults, duration, seed, warm/cold profile, and cleanup behavior.
- [ ] **V3-P00-T02** Define exact measurement methodology for API latency, runnable due-work delay, reconciliation lag, throughput, lock wait, database pool use, queue delivery, browser behavior, and cost.
- [ ] **V3-P00-T03** Define the independent safety oracles: simulator received/accepted journal, admission/reservation history, audit/replay results, and restore reconciliation state. FleetForge state alone cannot certify safety.
- [ ] **V3-P00-T04** Pin application/image/schema/IaC/tool versions, region, service sizing, telemetry config, database plan, scenario seed, budget, and sanitization check for every material run.
- [ ] **V3-P00-T05** Establish baseline acceptance rules and regression thresholds only after repeated valid baseline measurements; preserve unsuccessful but valid results.
- [ ] **V3-P00-T06** Set cost stop conditions, maximum instances/concurrency/storage/telemetry settings, runtime duration, cleanup hooks, and emergency disable procedure before every load/fault run.
- [ ] **V3-P00-T07** Prepare machine-readable result storage and a human report template with claim, exact result, caveat, bottleneck, environment, cost, and non-extrapolation statement.
- [ ] **V3-P00-T08** Review every public/resume claim against its planned evidence. Remove claims without a corresponding test/report path before starting validation.

## Exit gate

No v3 benchmark or fault run begins until another engineer could reproduce its configuration and determine whether a result passed, failed, or exceeded a stop condition.
