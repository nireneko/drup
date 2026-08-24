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

### Requirement: Patch URL Allowlist Validation

The system MUST validate every patch download URL against an allowlist using exact host matching (or a documented subdomain-suffix rule), and MUST require the `https` scheme. The system MUST NOT use substring containment (`strings.Contains`) to decide whether a URL's host is allowed.

| Req | Strength | Behavior |
|-----|----------|----------|
| Exact host match | MUST | Parse the URL and compare its host against the allowlist exactly, or against an explicit subdomain-suffix rule (`*.drupal.org`) |
| HTTPS only | MUST | Reject any URL whose scheme is not `https` |
| No substring match | MUST NOT | Decide allowlist membership via `strings.Contains(url, domain)` or equivalent |
| Reject on parse failure | MUST | Reject the URL if it fails to parse as a valid absolute URL |

#### Scenario: Accept legitimate Drupal.org URL

- GIVEN a patch URL `https://www.drupal.org/files/issues/2024-01-01/token-1234.patch`
- WHEN the allowlist check runs
- THEN the system SHALL accept the URL for download

#### Scenario: Reject host-appended-as-path bypass

- GIVEN a patch URL `https://evil.com/www.drupal.org/evil.patch`
- WHEN the allowlist check runs
- THEN the system SHALL reject the URL because its host is `evil.com`, not an allowed host

#### Scenario: Reject host-as-subdomain-of-attacker bypass

- GIVEN a patch URL `https://drupal.org.evil.com/evil.patch`
- WHEN the allowlist check runs
- THEN the system SHALL reject the URL because its host does not exactly match, and is not a subdomain of, an allowed host

#### Scenario: Reject allowlist-domain-in-query-string bypass

- GIVEN a patch URL `https://notdrupal.org/?x=git.drupal.org`
- WHEN the allowlist check runs
- THEN the system SHALL reject the URL because its host is `notdrupal.org`, not an allowed host

#### Scenario: Reject non-HTTPS scheme

- GIVEN a patch URL `http://drupal.org/files/issues/token-1234.patch`
- WHEN the allowlist check runs
- THEN the system SHALL reject the URL for using a disallowed scheme, regardless of host match

#### Scenario: Local path bypasses URL allowlist

- GIVEN a local filesystem patch path (not a URL)
- WHEN the system resolves the patch source
- THEN the local-path branch SHALL continue to apply without URL allowlist validation, unchanged from current behavior

### Requirement: Git Apply

The system SHALL apply downloaded patches using `git apply`. The system SHALL determine the project web root by reading `composer.json` → `extra.drupal-scaffold.locations.web-root` and use it as the base path for patch operations. If the scaffold config is not present, the system SHALL fall back to `web/` as the default web root. The system MUST NOT use `os.Getwd()` to determine the web root.

| Req | Strength | Behavior |
|-----|----------|----------|
| Web root from composer | MUST | Read `extra.drupal-scaffold.locations.web-root` from `composer.json` |
| Fallback | MUST | Default to `web/` if scaffold config absent |
| No os.Getwd() | MUST NOT | Use `os.Getwd()` for web root determination |
| Project path based | MUST | Resolve web root relative to `project_path` parameter |

(Previously: used `os.Getwd()` which fails when drup runs from a different working directory than the Drupal project root)

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

#### Scenario: Custom web root from composer scaffold

- GIVEN `composer.json` with `extra.drupal-scaffold.locations.web-root: "docroot"`
- WHEN create_patch resolves the web root
- THEN the system SHALL use `<project_path>/docroot` as the base path

#### Scenario: No scaffold config present

- GIVEN `composer.json` without `extra.drupal-scaffold`
- WHEN create_patch resolves the web root
- THEN the system SHALL fall back to `<project_path>/web`

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
