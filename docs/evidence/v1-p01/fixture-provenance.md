# Virtual-BMC fixture provenance

Status: **frozen for the synthetic virtual-BMC route contract.**

| Required record | Value |
| --- | --- |
| Official DMTF release and publication date | Redfish 2026.2, published 16 September 2026; DSP8010 2026.2, DSP0268 2026.2, DSP0266 1.25.0 |
| Selected schema source URLs and checksums | `test/fixtures/redfishsim/v1/schema-manifest.json`, immutable Git commit `4f81814e399213c9055863ddaff42791c6b3706a` |
| Schema/fixture license and intended evidence-use permission | DMTF repository BSD-3-Clause, SHA-256 `30733d426c211a281f673aa726b3ec08827e4f8fc9fd8a93936d30fe9644c6d4`; only checksums/provenance are committed |
| Scenario schema version and digest | Existing simulator scenario contract remains `simulator.scenario.v1`; route fixtures are independent of run scenarios |
| Canonical Service Root, identity, update, task, inventory, event, and error fixtures | `test/fixtures/redfishsim/v1/*.json` |
| Simulator extension field/header classification | Every fixture classifies selected standard, `Oem.FleetForge`, and admin-only fields; synthetic update headers are discovery metadata only |
| Local target-auth and separate admin-auth procedure | Credentials are process configuration, distinct by listener role, and prohibited from fixtures, journals, and diagnostics |
| Synthetic update and crash/recovery evidence | Deferred to lifecycle and recovery runs; no route fixture asserts hardware behavior |
| Redaction review | Fixture validation rejects credential material; only loopback callback example addresses are present |

No value in this template proves external-device compatibility. The fixture
bundle must be accepted by the virtual-BMC contract spike before it freezes an
HTTP route, request body, content type, timing, task behavior, or recovery
behavior.
