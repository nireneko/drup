# Delta for Scan

## ADDED Requirements

### Requirement: Evidence Hash

The system SHALL compute a SHA256 hash over the normalized, serialized findings of a scan result and include it as `evidence_hash` in the scan output.

#### Scenario: Deterministic hash for identical findings

- GIVEN two scan runs producing byte-identical normalized findings
- WHEN each run computes `evidence_hash`
- THEN both hashes SHALL be equal

#### Scenario: Different findings produce different hash

- GIVEN two scan runs whose findings differ in at least one error entry
- WHEN each run computes `evidence_hash`
- THEN the hashes SHALL differ

#### Scenario: Empty findings still produce a hash

- GIVEN a scan run with zero errors
- WHEN `evidence_hash` is computed
- THEN the system SHALL return a valid hash over the empty/zero-error normalized structure, not an empty or omitted field
