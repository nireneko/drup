# Gitops Specification

## Purpose

Git operations for upgrade automation: clean verification, atomic commits, and branch management.

## Requirements

### Requirement: Git Clean Verification

The system SHALL verify the git working tree is clean before performing commits.

#### Scenario: Clean tree before commit

- GIVEN a clean git working tree (no staged, unstaged, or untracked changes)
- WHEN gitops clean check runs
- THEN the system SHALL return clean status and allow proceeding

#### Scenario: Dirty tree before commit

- GIVEN uncommitted changes exist
- WHEN gitops clean check runs
- THEN the system SHALL return dirty status with the list of changed files

### Requirement: Atomic Commits

The system SHALL create one commit per unit of work with conventional commit format.

#### Scenario: Commit after rector run

- GIVEN rector has modified files in custom modules
- WHEN gitops creates a commit
- THEN the system SHALL stage only rector-modified files and commit with message `refactor: apply drupal-rector to <module>`

#### Scenario: Commit after contrib patch

- GIVEN a patch was applied to contrib module `token`
- WHEN gitops creates a commit
- THEN the system SHALL stage only token-related files and commit with message `fix(contrib): apply D11 patch to token`

#### Scenario: Commit after custom file fix

- GIVEN a custom module file was fixed
- WHEN gitops creates a commit
- THEN the system SHALL stage only that file and commit with message `fix(custom): resolve deprecation in <file>`

### Requirement: Branch Management

The system SHALL create and manage an `upgrade/drupal-11` branch for all upgrade work.

#### Scenario: Create upgrade branch

- GIVEN the project is on `main` branch with a clean tree
- WHEN gitops initializes the upgrade branch
- THEN the system SHALL create and checkout `upgrade/drupal-11`

#### Scenario: Branch already exists

- GIVEN `upgrade/drupal-11` already exists
- WHEN gitops initializes the upgrade branch
- THEN the system SHALL checkout the existing branch without recreating it

### Requirement: Commit Verification

The system SHALL verify each commit was created successfully by checking the commit hash.

#### Scenario: Successful commit

- GIVEN files are staged and `git commit` runs
- WHEN the commit succeeds
- THEN the system SHALL capture and return the commit hash

#### Scenario: Commit fails (empty)

- GIVEN no files are staged
- WHEN `git commit` is attempted
- THEN the system SHALL report "nothing to commit" and not create an empty commit
