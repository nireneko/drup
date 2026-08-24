# Delta for Apply Patch

## ADDED Requirements

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
