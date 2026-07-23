# Pipeline Metrics Specification

## Purpose

Non-blocking metrics collection throughout the upgrade pipeline for observability and reporting.

## Requirements

### Requirement: Non-Blocking Metrics Collection

The system SHALL collect pipeline execution metrics throughout the upgrade process. Metrics collection MUST be non-blocking — a metrics failure SHALL NOT halt or affect the pipeline. The system SHALL output metrics as a JSON section in the final report.

| Req | Strength | Behavior |
|-----|----------|----------|
| Total duration | MUST | Track `total_duration_ms` from pipeline start to end |
| Stage durations | MUST | Track per-stage `duration_ms` in `stage_durations` map |
| Commands executed | MUST | Count total shell commands in `commands_executed` |
| Files modified | MUST | Count files changed in `files_modified` |
| Retries | MUST | Count retry attempts in `retries` |
| Human interventions | MUST | Count human escalations in `human_interventions` |
| Non-blocking | MUST | Metrics collection failure SHALL NOT block pipeline |
| JSON output | MUST | Output as `pipeline_metrics` section in report |

#### Scenario: Full pipeline metrics collection

- GIVEN a complete pipeline run with all stages
- WHEN the pipeline finishes
- THEN the system SHALL produce `{total_duration_ms: N, stage_durations: {preflight: N, scan: N, ...}, commands_executed: N, files_modified: N, retries: N, human_interventions: N}`

#### Scenario: Metrics collection error

- GIVEN a metrics tracking error (e.g., clock skew, counter overflow)
- WHEN the pipeline runs
- THEN the system SHALL log the metrics error and continue pipeline execution normally

#### Scenario: Partial pipeline run

- GIVEN a pipeline that fails at Stage 3 (contrib)
- WHEN metrics are collected
- THEN the system SHALL report durations only for completed stages and include the failure point
