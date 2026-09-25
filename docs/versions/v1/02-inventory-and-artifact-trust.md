# V1-P02 — Target Inventory, Preflight, and Artifact Trust

## Goal

Make a v1 campaign eligible only when its simulator target set and synthetic artifact are explicit, immutable, policy-checked, and safe to reference.

## Tasks

- [ ] **V1-P02-T01** Define target registration as an application command with external ID, canonical endpoint identity, bounded labels, and a provider-neutral credential reference. The local adapter resolves only synthetic test references; the later AWS adapter uses Secrets Manager. Reject credentials in URLs and API/browser storage.
- [ ] **V1-P02-T02** Implement endpoint canonicalization and SSRF controls: allowlisted scheme/host/IP/CIDR/port, no redirects, bounded DNS behavior, and validation of every device-returned task/resource URI.
- [ ] **V1-P02-T03** Define the v1 Redfish adapter port and normalized discovery errors. Set connect, TLS, response-header, body-read, and total deadlines plus body/page/decompression bounds.
- [ ] **V1-P02-T04** Persist normalized identity, current firmware, capabilities, reachability, last-known-good observation, freshness, and safe raw-evidence hash/reference. Do not persist secrets or unbounded documents.
- [ ] **V1-P02-T05** Schedule discovery through durable work; deduplicate active discovery per target revision and classify proven transient versus stale/incompatible conditions.
- [ ] **V1-P02-T06** Implement a deliberately small selector grammar for exact labels and bounded conjunctions. Compile only parameterized SQL, deterministic ordering, and bounded result size; document unsupported operators.
- [ ] **V1-P02-T07** Implement preflight with stable per-target/global codes. Freeze explicit target IDs, revisions, rack labels, relevant capabilities, selector/compiler versions, and a canonical snapshot hash.
- [ ] **V1-P02-T08** Recheck active enrollment and unresolved-operation quarantine in the transaction that moves a campaign to `READY`; one blocked target must roll back the whole readiness attempt.
- [ ] **V1-P02-T09** Define v1 synthetic-artifact fields and lifecycle. Bind provider, bucket/container, key, assigned version ID, size, SHA-256, declared version, target-family compatibility, and verification-policy version immutably.
- [ ] **V1-P02-T10** Implement authorized direct upload initiation through the object-store port. For S3, presign one short-lived PUT for the exact new key, expected checksum, content type, and permitted headers; after upload, atomically bind the non-null version ID returned by the versioned bucket before verification. The API never proxies artifact bytes or returns bucket-wide permission.
- [ ] **V1-P02-T11** Claim artifact verification as durable work; head and stream exactly the bound version ID through bounded SHA-256 verification. Reject mismatch, oversized input, and unsupported required signature format.
- [ ] **V1-P02-T12** Persist a stable verification result and enforce that retired/rejected/expired artifacts cannot make new campaigns ready. Existing campaign evidence remains immutable.
- [ ] **V1-P02-T13** Evaluate the simple compatibility policy against every frozen target and store its decision/version in the campaign snapshot.
- [ ] **V1-P02-T14** Add bounded cleanup only for abandoned uploads and a dry-run orphan report. Do not age-delete verified referenced objects.

## Required proof

- Loopback/private endpoint outside policy, redirect, oversized response, malformed task URI, and malicious metadata fail before unsafe I/O.
- Same registration/semantic request is idempotent; changed endpoint conflicts.
- Repeated selector evaluation yields the same ordered IDs and snapshot hash; post-freeze inventory drift changes neither membership nor artifact identity.
- Replacing an object key with another version, digest mismatch, duplicate verification, and artifact-retirement race are rejected or stable in the local contract adapter; live S3 behavior remains a later provider gate.

## Exit gate

A validated campaign can reference only a frozen, conflict-free simulator target snapshot and one verified immutable object version. No firmware submission exists yet.
