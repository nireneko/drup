# Delta for Gitops

## ADDED Requirements

### Requirement: Scoped Commit Helper

The system SHALL provide a scoped-commit helper that stages only an explicitly declared list of paths and MUST verify, before committing, that the staged file set is a subset of that declared list. The system MUST NOT use `git add -A` (or equivalent stage-everything operations) for any automated commit.

#### Scenario: Commit with all staged files declared

- GIVEN a caller declares an explicit list of allowed paths and only those paths have pending changes
- WHEN the scoped-commit helper runs
- THEN the system SHALL stage exactly the declared paths and create the commit

#### Scenario: Unexpected staged file aborts the commit

- GIVEN the working tree has a changed file outside the declared allowed-path list
- WHEN the scoped-commit helper runs
- THEN the system SHALL abort before committing and SHALL report the unexpected path(s) in the error

#### Scenario: Empty declared list

- GIVEN no paths are declared for a commit
- WHEN the scoped-commit helper runs
- THEN the system SHALL report "nothing to commit" and SHALL NOT create an empty commit
