# Remaining simulator and Phase 04 work

The external capability gate has been removed. The following work remains
ordered behind the accepted synthetic contract fixtures and recovery design:

- Redfish-shaped listener routes, selected properties, schema contracts,
  update media type, task shape, inventory evidence, and Basic authentication.
- Any accepted simulated firmware update, reboot lifecycle, callback protocol,
  or update-specific fault injection.
- Pinned DMTF schema/fixture provenance, synthetic update replay, recovery, and
  independent-oracle proof.
- Optional container deployment and local/CI smoke tests; no simulator cloud VM
  is required.
- RF-F09, RF-F12, RF-F13, RF-F14, RF-F15, full 1,000-target measurement, and
  Phase 04 load proof.

Implementation proceeds only from the accepted contract and canonical
fixtures. Virtual-BMC evidence cannot establish vendor, external-device, or
complete Redfish behavior.
