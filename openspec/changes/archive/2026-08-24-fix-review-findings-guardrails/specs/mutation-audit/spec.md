# Mutation Audit Specification

## Purpose

Persistent, cross-run JSONL ledger of every mutating MCP tool call, enforced caps on mutation volume, and a read-only status tool for inspecting ledger state.

## Requirements

### Requirement: Mutation Ledger

The system SHALL append one JSONL record per mutating tool invocation to a per-project ledger file, using the atomic tmp-file+rename write pattern. Each record SHALL contain: tool name, SHA256 hash of the call's raw arguments, result status, commit hash (when applicable), and a timestamp.

#### Scenario: Successful mutation logged

- GIVEN a mutating tool call completes with a resulting commit
- WHEN the ledger writer runs
- THEN it SHALL append a record with `result: "success"` and the commit hash

#### Scenario: Failed mutation logged

- GIVEN a mutating tool call fails before producing a commit
- WHEN the ledger writer runs
- THEN it SHALL append a record with `result: "failure"` and an empty/absent commit hash

#### Scenario: Ledger write does not block the tool response

- GIVEN the ledger file is temporarily unwritable
- WHEN a mutating tool call otherwise completes
- THEN the system SHALL still return the tool's result and SHALL log the ledger write failure separately

### Requirement: Mutation Caps

The system SHALL enforce a configurable cap on the number of mutating calls per session (or per day, when no session is required per `agent-session` opt-out). A safe default cap SHALL apply when no configuration is present.

#### Scenario: Cap reached

- GIVEN the configured or default cap has been reached for the current session
- WHEN another mutating tool is called
- THEN the system SHALL refuse the call with an error naming the cap and the current count

#### Scenario: Cap not configured

- GIVEN no per-project cap configuration exists
- WHEN mutating calls are made
- THEN the system SHALL apply the built-in safe default cap rather than allowing unlimited calls

### Requirement: pipeline_status Tool

The system SHALL expose a read-only `pipeline_status` MCP tool summarizing ledger counts by tool and remaining cap headroom for the current project.

#### Scenario: Status with prior mutations

- GIVEN the ledger contains records for the current project
- WHEN `pipeline_status({project_path: "/path"})` is called
- THEN the system SHALL return per-tool counts, total mutations, and remaining cap

#### Scenario: Status on empty ledger

- GIVEN no ledger file exists yet for the project
- WHEN `pipeline_status` is called
- THEN the system SHALL return zero counts and the full cap as remaining, without erroring
