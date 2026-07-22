# Patch Reconcile Specification

## Purpose

Deterministic capability to keep applied community patches current: detect newer versions of an applied patch, verify whether a patch is still needed, and adapt/create a local patch when the upstream one no longer applies — while preserving the original issue/PR reference. Invoked by a dispatched sub-agent; the orchestrator only reads its report.

## Requirements

### Requirement: Newer Patch Detection

The system SHALL check the source drupal.org issue for a patch revision newer than the one currently applied.

#### Scenario: Newer patch exists

- GIVEN a module has patch `issue-123-v2.patch` applied
- WHEN the sub-agent checks the issue queue
- THEN the system SHALL report that `issue-123-v3.patch` is available with its issue URL

#### Scenario: No newer patch

- GIVEN the applied patch is the latest revision on the issue
- WHEN the sub-agent checks the issue queue
- THEN the system SHALL report the patch as current

### Requirement: Patch Still-Needed Verification

The system SHALL verify whether the code change carried by an applied patch is already present in the module's current shipped code (e.g., merged upstream in a newer release).

#### Scenario: Patch already merged upstream

- GIVEN the module's latest release already contains the patched code
- WHEN the sub-agent verifies necessity
- THEN the system SHALL report the patch as obsolete and safe to remove from `composer.json` patches

#### Scenario: Patch still needed

- GIVEN the module's latest release does not contain the patched code
- WHEN the sub-agent verifies necessity
- THEN the system SHALL report the patch as still required

### Requirement: Local Patch Adaptation

The system SHALL adapt or regenerate a local patch when the upstream patch fails to apply cleanly against the current module version.

#### Scenario: Upstream patch no longer applies cleanly

- GIVEN a patch produces rejects against the current module version
- WHEN the sub-agent attempts reconciliation
- THEN the system SHALL generate an adapted local patch reproducing the same intent and report it as locally adapted

### Requirement: Issue Reference Preservation

The system SHALL preserve the original drupal.org issue or PR reference in any adapted or newly applied patch entry.

#### Scenario: Reference kept after adaptation

- GIVEN a local patch is adapted from an upstream patch tied to issue `#3123456`
- WHEN the system writes the new patch entry
- THEN the `composer.json` patches description and patch file header SHALL both reference issue `#3123456`
