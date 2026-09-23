# Deterministic simulator evidence

This directory contains redacted, reproducible simulator evidence and contract
templates. It contains no QEMU response, credential, session token, firmware
image, signed URL, real endpoint, or AWS account data.

Current status: **preparation only**. The pinned QEMU/OpenBMC capability
fixture and accepted update route have not been supplied, so this directory
does not establish Redfish/OpenBMC compatibility or a firmware-update claim.

| Artifact | Purpose | Current status |
| --- | --- | --- |
| `supported-resource-matrix.md` | Route/property contract and source classification | Blocked pending fixture |
| `fixture-provenance.md` | Redacted fixture identity and license record | Blocked pending selected image |
| `scenario-and-fault-catalog.md` | Simulator-only scenario and fault boundary | Prepared |
| `deferred-phase04-work.md` | Explicitly deferred simulator and Phase 04 claims | Active |

Each later evidence report must name the repository commit, Go version,
scenario digest, seed, trace digest, test command, execution environment, and
limitations. Large raw journals remain bounded CI artifacts; only small,
redacted canonical fixtures may be committed.
