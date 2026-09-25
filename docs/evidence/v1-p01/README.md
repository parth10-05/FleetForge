# Deterministic simulator evidence

This directory contains redacted, reproducible simulator evidence and contract
templates. It contains no live credential, session token, executable firmware
image, signed URL, external endpoint, or AWS account data.

Current status: **route-independent preparation implemented; synthetic
contract refreeze approved.** Canonical route fixtures and the accepted update
path must now be completed. This directory establishes only repository-owned
synthetic-target behavior and no external-device compatibility claim.

| Artifact | Purpose | Current status |
| --- | --- | --- |
| `supported-resource-matrix.md` | Route/property contract and source classification | Refreeze required |
| `fixture-provenance.md` | Selected schema and canonical fixture provenance | Refreeze required |
| `scenario-and-fault-catalog.md` | Simulator-only scenario and fault boundary | Prepared |
| `deferred-phase04-work.md` | Remaining route-specific implementation and claim limits | Active |
| `implementation-status.md` | Requirement-level implementation and blocking record | Current |

Each later evidence report must name the repository commit, Go version,
scenario digest, seed, trace digest, test command, execution environment, and
limitations. Large raw journals remain bounded CI artifacts; only small,
redacted canonical fixtures may be committed.
