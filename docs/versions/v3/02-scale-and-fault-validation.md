# V3-P02 — Scale and Fault Validation

## Goal

Measure the simulator-backed control plane against the declared 1,000-target design target and adversarial schedules. Keep the claim limited to the exact workload and environment tested.

## Tasks

- [ ] **V3-P02-T01** Run L0 smoke and repeated L1 functional baselines; verify metrics/traces, test isolation, cleanup, and variance before scaling.
- [ ] **V3-P02-T02** Run L2 with 1,000 targets, 25 global concurrency, and 2/rack without injected terminal failures. Capture latency distributions, throughput, pool/lock/queue data, reservations, browser profile, and cost.
- [ ] **V3-P02-T03** Identify the first bottleneck from evidence, change one variable at a time, and retain before/after reports rather than attributing improvement to a bundle of tuning changes.
- [ ] **V3-P02-T04** Run L3 mixed faults: lost/duplicate/out-of-order events, reboots, task loss, slow targets, `429/503`, telemetry disruption, queue delivery disruption, and database availability loss.
- [ ] **V3-P02-T05** Run L4 canary stop with a systemic bad artifact. Prove zero new later-wave admission after the pause commit and separately enumerate pre-commit admitted residual exposure.
- [ ] **V3-P02-T06** Run L5 correlated-failure storm. Measure jitter, retry budgets, circuit behavior, queue/DB pressure, outbox/due-work age, and bounded recovery without synchronized retry amplification.
- [ ] **V3-P02-T07** Inject worker kill/stall at every documented persistence/network boundary, including an original submitter alive after lease expiry; preserve failing seeds and prove no duplicate accepted operation.
- [ ] **V3-P02-T08** Verify global/site/rack physical reservations, fairness under a hot rack, queue/connection caps, and no unbounded goroutine/memory/backlog behavior throughout the runs.
- [ ] **V3-P02-T09** Run a budget-compatible L6 soak and report resource growth, stale work, retention/cleanup, connection leaks, UI stability, and observed cost.
- [ ] **V3-P02-T10** Publish the scale/fault report with raw result references, all failed valid runs, exact accepted-operation oracle output, bottleneck, tuning history, limitations, and non-extrapolation wording.

## Exit gate

Every required L2–L5 safety assertion passes at the recorded commit. The project may state only the measured 1,000-target simulator result; it may not claim an untested fleet size, hardware behavior, or availability SLO.
