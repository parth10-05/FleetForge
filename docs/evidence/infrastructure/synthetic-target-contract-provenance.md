# Synthetic target contract provenance

Status: **contract refreeze required before route implementation**

| Field | Required record |
|---|---|
| DMTF release | Exact release identifier and publication date |
| Selected schemas | Resource names, versions, source URLs, and checksums |
| Synthetic contract ADRs | Accepted ADR-032, ADR-038, and ADR-039 revisions |
| Canonical fixtures | Repository paths and fixture digests |
| Scenario contract | Schema version, limits, and canonical scenario digests |
| Update contract | Route, media type, extension headers, byte/digest/version limits |
| Task and observation contract | State mappings, timing, reboot, inventory, health, events |
| Authentication | Local target-boundary method and separate admin-secret procedure |
| Recovery | Owner lock, replay, snapshot, invalidation, and readiness procedure |
| Claim boundary | No external device, vendor, or complete Redfish compatibility claim |

This record proves only the provenance of FleetForge's repository-owned
synthetic target contract. It is not evidence of external-device behavior.
