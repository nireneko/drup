# Drupal Custom D11 Fixes Skill Specification

## Purpose

A skill providing a catalog of Drupal 11 deprecation patterns with actionable fix guidance.

## Requirements

### Requirement: D11 Deprecation Catalog Skill

A skill file SHALL exist at `skills/drupal-custom-d11-fixes/SKILL.md` containing a catalog of approximately 50 Drupal 11 deprecation patterns. Each pattern MUST include: the deprecation description, replacement API, before/after code examples, complexity rating, and edge cases.

| Req | Strength | Behavior |
|-----|----------|----------|
| Location | MUST | `skills/drupal-custom-d11-fixes/SKILL.md` |
| Pattern count | SHOULD | ~50 patterns covering common D11 deprecations |
| Pattern structure | MUST | Each entry: deprecation, replacement, before/after, complexity, edge cases |
| Trigger | MUST | Activate when drup fix/validate finds custom module deprecations |

#### Scenario: Skill loads correctly

- GIVEN the skill file exists at the expected path
- WHEN the agent loads the skill
- THEN the skill SHALL provide deprecation patterns with actionable fix guidance

#### Scenario: Skill triggers on custom deprecation

- GIVEN `drup validate` reports deprecations in `web/modules/custom/mymodule/`
- WHEN the agent needs fix guidance
- THEN the agent SHALL load this skill and match the deprecation to a catalog entry
