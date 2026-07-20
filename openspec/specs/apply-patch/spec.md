# Apply Patch Specification

## Purpose

Download patch files, apply them via git, and register them in composer-patches configuration.

## Requirements

### Requirement: Patch Download

The system SHALL download patch files from URLs using `net/http`.

#### Scenario: Download from Drupal.org

- GIVEN a valid patch URL on Drupal.org
- WHEN the system downloads it
- THEN the system SHALL save the .patch file to a temporary location and return the local path

#### Scenario: Download fails (404)

- GIVEN a patch URL that returns 404
- WHEN the system attempts download
- THEN the system SHALL return an error with the HTTP status code

#### Scenario: Download fails (network error)

- GIVEN a network failure during download
- WHEN the system attempts download
- THEN the system SHALL return a connection error without leaving partial files

### Requirement: Git Apply

The system SHALL apply downloaded patches using `git apply`.

#### Scenario: Clean apply

- GIVEN a valid .patch file and a clean git working tree
- WHEN the system runs `git apply <patch_file>`
- THEN the system SHALL report `{applied: true}` with the list of modified files

#### Scenario: Apply conflict

- GIVEN a patch that conflicts with current code
- WHEN the system runs `git apply <patch_file>`
- THEN the system SHALL report `{applied: false}` with the conflict details from git stderr

#### Scenario: Apply with whitespace issues

- GIVEN a patch with whitespace differences
- WHEN the system runs `git apply --whitespace=nowarn <patch_file>`
- THEN the system SHALL attempt apply with whitespace tolerance before reporting failure

### Requirement: Composer-Patches Registration

The system SHALL register applied patches in `composer.json` under `extra.patches` using the cweagans/composer-patches format.

#### Scenario: Register new patch

- GIVEN a successfully applied patch for module `token`
- WHEN the system updates composer.json
- THEN the system SHALL add an entry under `extra.patches.drupal/token` with the patch description and URL

#### Scenario: Module already has patches

- GIVEN `extra.patches.drupal/token` already contains entries
- WHEN a new patch is registered
- THEN the system SHALL append to the existing array without removing prior entries

#### Scenario: No extra.patches key exists

- GIVEN a `composer.json` without `extra.patches`
- WHEN the first patch is registered
- THEN the system SHALL create the `extra.patches` structure and add the entry

### Requirement: Atomic Operation

The system SHALL treat download + apply + register as an atomic unit — if any step fails, previously applied changes SHALL be reverted.

#### Scenario: Apply fails after download

- GIVEN a patch was downloaded but `git apply` fails
- WHEN the failure is detected
- THEN the system SHALL clean up the temporary patch file and report the error without modifying composer.json

#### Scenario: Registration fails after apply

- GIVEN a patch was applied but composer.json update fails
- WHEN the failure is detected
- THEN the system SHALL revert the git apply (`git apply -R`) and clean up
