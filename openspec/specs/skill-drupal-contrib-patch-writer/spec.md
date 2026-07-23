# Drupal Contrib Patch Writer Skill Specification

## Purpose

A skill providing guidelines for writing minimal contrib patches organized by error category.

## Requirements

### Requirement: Contrib Patch Writer Skill

A skill file SHALL exist at `skills/drupal-contrib-patch-writer/SKILL.md` containing guidelines for writing minimal contrib patches organized by error category. Categories MUST include: (A) info.yml fixes, (B) simple replacements, (C) API parameter changes, (D) architecture changes (escalate to human).

| Req | Strength | Behavior |
|-----|----------|----------|
| Location | MUST | `skills/drupal-contrib-patch-writer/SKILL.md` |
| Category A | MUST | info.yml fixes (core_version_requirement, etc.) |
| Category B | MUST | Simple text replacements (renamed functions, constants) |
| Category C | MUST | API parameter changes (new/removed params) |
| Category D | MUST | Architecture changes — escalate, do not auto-patch |

#### Scenario: Skill guides patch for info.yml fix

- GIVEN a contrib module with outdated `core_version_requirement`
- WHEN the agent writes a patch
- THEN the skill SHALL guide a Category A fix with minimal diff

#### Scenario: Skill escalates architecture changes

- GIVEN a contrib module requiring service container restructuring
- WHEN the agent evaluates the fix
- THEN the skill SHALL direct escalation to human review (Category D)
