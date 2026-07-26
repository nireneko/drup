# Drupal.org Resilience Specification

## Purpose

Provide HTTP retry and exponential backoff for Drupal.org API calls to handle transient failures (412 Precondition Failed, timeouts) without manual intervention.

## Requirements

### Requirement: Retry on Transient Failures

The system SHALL retry Drupal.org HTTP requests that fail with transient errors, using exponential backoff.

The system SHALL attempt up to 3 requests total (1 initial + 2 retries).

The system SHALL use a base delay of 500ms, doubling on each retry (500ms, 1000ms).

#### Scenario: Request succeeds on first attempt

- GIVEN a Drupal.org API endpoint is reachable
- WHEN the system makes an HTTP request
- THEN the system SHALL return the response on the first attempt
- AND no retries SHALL occur

#### Scenario: Request fails with 412 then succeeds

- GIVEN a Drupal.org API returns 412 on the first attempt
- WHEN the system retries
- THEN the system SHALL wait 500ms before the second attempt
- AND if the second attempt succeeds, the system SHALL return that response

#### Scenario: Request times out then succeeds

- GIVEN a Drupal.org API request times out on the first attempt
- WHEN the system retries
- THEN the system SHALL wait 500ms before the second attempt
- AND if the second attempt succeeds, the system SHALL return that response

#### Scenario: All retries exhausted

- GIVEN a Drupal.org API fails on all 3 attempts
- WHEN the system exhausts retries
- THEN the system SHALL return the last error to the caller
- AND the system SHALL log each retry attempt with the attempt number and delay

### Requirement: Retryable Error Classification

The system SHALL classify HTTP 412, 429, 500, 502, 503, 504, and timeout errors as retryable. All other HTTP errors SHALL NOT be retried.

#### Scenario: Non-retryable error returns immediately

- GIVEN a Drupal.org API returns HTTP 404
- WHEN the system makes the request
- THEN the system SHALL return the 404 error immediately
- AND the system SHALL NOT retry

#### Scenario: HTTP 429 is retried

- GIVEN a Drupal.org API returns HTTP 429
- WHEN the system makes the request
- THEN the system SHALL retry with exponential backoff

### Requirement: Retry Logging

The system SHALL log each retry attempt with the attempt number, delay duration, and error received.

#### Scenario: Retry logged on each attempt

- GIVEN a request fails and is retried
- WHEN each retry occurs
- THEN the system SHALL emit a log entry with attempt number (2, 3), delay, and error
- AND the final failure SHALL include all retry attempts in the error context
