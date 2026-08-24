# Delta for Validation Gates

## ADDED Requirements

### Requirement: Evidence Hash Verification

The `validate` MCP tool SHALL accept an optional `expected_hash` parameter. When provided, the system MUST fail closed: if the current scan's `evidence_hash` does not match `expected_hash`, validation MUST be reported as failed regardless of `total_errors`.

#### Scenario: Matching expected_hash

- GIVEN `validate({project_path: "/path", expected_hash: "<hash>"})` where `<hash>` matches the current scan's `evidence_hash`
- WHEN validation runs
- THEN the system SHALL proceed with normal `total_errors`-based gating

#### Scenario: Mismatched expected_hash fails closed

- GIVEN `validate({project_path: "/path", expected_hash: "<stale-hash>"})` where the current scan's `evidence_hash` differs
- WHEN validation runs
- THEN the system SHALL report validation as failed, even if `total_errors == 0`, and SHALL include both hashes in the error

#### Scenario: expected_hash omitted preserves prior behavior

- GIVEN `validate({project_path: "/path"})` with no `expected_hash`
- WHEN validation runs
- THEN the system SHALL gate solely on `total_errors`, unchanged from current behavior
