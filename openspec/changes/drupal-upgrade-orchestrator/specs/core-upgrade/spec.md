# Core Upgrade Specification

## Purpose

Deterministic capability to check for and apply the next Drupal core major version bump in `composer.json`, with dry-run inspection and rollback safety. Invoked by a dispatched sub-agent (not the orchestrator directly); the orchestrator only reads the sub-agent's report.

## Requirements

### Requirement: Next Major Version Check

The system SHALL check the latest stable `drupal/core` release for the next major version relative to the project's currently installed core version.

#### Scenario: Next major available

- GIVEN a project on Drupal 10.x
- WHEN the sub-agent runs the core-upgrade check
- THEN the system SHALL report the latest available Drupal 11.x release and its constraint string

#### Scenario: Already on latest major

- GIVEN a project already on the newest available major version
- WHEN the sub-agent runs the core-upgrade check
- THEN the system SHALL report no next major is available

### Requirement: Dry-Run Validation Before Apply

The system SHALL support a dry-run mode that reports the exact `composer.json` changes it would make without writing to disk.

#### Scenario: Dry-run shows diff only

- GIVEN a next major version is available
- WHEN the sub-agent runs a dry-run
- THEN the system SHALL return the proposed `composer.json` diff and SHALL NOT modify any file

### Requirement: Clean Tree Precondition

The system SHALL require a clean git working tree before applying a core version change.

#### Scenario: Dirty tree blocks apply

- GIVEN uncommitted changes exist in the working tree
- WHEN the sub-agent attempts to apply the core version bump
- THEN the system SHALL refuse to apply and report the dirty-tree condition

### Requirement: Composer.json Update

The system SHALL update the `drupal/core` constraint in `composer.json` to the next major version and create a git checkpoint commit immediately before the change.

#### Scenario: Apply succeeds

- GIVEN a clean working tree and an available next major version
- WHEN the sub-agent applies the core version bump
- THEN the system SHALL create a pre-change checkpoint commit, update `composer.json`, and report the new constraint

### Requirement: Rollback Capability

The system SHALL provide a rollback that restores `composer.json` (and `composer.lock` if changed) to the pre-apply checkpoint commit.

#### Scenario: Rollback after failed follow-up install

- GIVEN a core version bump was applied and checkpointed
- WHEN a subsequent `composer install` fails and rollback is requested
- THEN the system SHALL revert to the checkpoint commit and report the restored state
