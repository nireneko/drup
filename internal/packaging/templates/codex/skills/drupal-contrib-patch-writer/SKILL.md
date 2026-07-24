---
name: drupal-contrib-patch-writer
description: "Guidelines for writing minimal contrib module patches organized by error category. Trigger: drup validate finds deprecations in contrib modules."
version: "1.0.0"
---

# Drupal Contrib Patch Writer

Write minimal, targeted patches for contrib modules during D10→D11 upgrades. Organized by error category — each category has a different patching strategy and risk level.

## Decision Tree

```
Deprecation found in contrib module
  │
  ├─ Category A: info.yml fix → Auto-patch (safe, minimal diff)
  ├─ Category B: Simple replacement → Auto-patch (low risk)
  ├─ Category C: API parameter change → Auto-patch with review (medium risk)
  └─ Category D: Architecture change → ESCALATE to human (high risk)
```

---

## Category A: info.yml Fixes

**Risk**: None — metadata only, no code changes.
**Patch size**: 1-3 lines.
**Auto-patch**: YES.

### A1: core_version_requirement

```yaml
# Before
core_version_requirement: ^10

# After
core_version_requirement: ^10 || ^11
```

### A2: Missing type key in info.yml

```yaml
# Before
name: My Module
core_version_requirement: ^10

# After
name: My Module
type: module
core_version_requirement: ^10 || ^11
```

### A3: Deprecated package in composer.json

```json
// Before
"drupal/core": "^10.3"

// After
"drupal/core": "^10.3 || ^11"
```

---

## Category B: Simple Replacements

**Risk**: Low — straightforward text substitution.
**Patch size**: 1-10 lines.
**Auto-patch**: YES.

### B1: Deprecated function → Service call

```php
// Before
drupal_set_message('Done!', 'status');

// After
\Drupal::messenger()->addMessage('Done!', 'status');
```

### B2: Deprecated static method → Service

```php
// Before
$url = file_create_url($uri);

// After
$url = \Drupal::service('file_url_generator')->generateString($uri);
```

### B3: Deprecated entity function → EntityTypeManager

```php
// Before
$node = entity_load('node', $nid);

// After
$node = \Drupal::entityTypeManager()->getStorage('node')->load($nid);
```

### B4: Deprecated Unicode utility → mbstring

```php
// Before
$len = Unicode::strlen($text);

// After
$len = mb_strlen($text);
```

### B5: Deprecated database function → Connection

```php
// Before
$result = db_query('SELECT * FROM {node} WHERE nid = :nid', [':nid' => $nid]);

// After
$result = \Drupal::database()->query('SELECT * FROM {node} WHERE nid = :nid', [':nid' => $nid]);
```

### B6: Deprecated render function → Renderer

```php
// Before
$output = drupal_render($build);

// After
$output = \Drupal::service('renderer')->render($build);
```

### B7: Deprecated date function → DateFormatter

```php
// Before
$date = format_date($timestamp, 'custom', 'Y-m-d');

// After
$date = \Drupal::service('date.formatter')->format($timestamp, 'custom', 'Y-m-d');
```

---

## Category C: API Parameter Changes

**Risk**: Medium — may change behavior subtly.
**Patch size**: 5-20 lines.
**Auto-patch**: YES, but flag for review.

### C1: Method signature changed

```php
// Before
$result = $service->process($data, $options);

// After
$result = $service->process($data, $options, $new_default_param);
```

**Note**: Verify the new parameter's default value matches expected behavior.

### C2: Return type changed

```php
// Before
$items = $storage->loadMultiple($ids); // Returns array
foreach ($items as $item) { ... }

// After
$items = $storage->loadMultiple($ids); // May return different type
if (is_array($items)) {
    foreach ($items as $item) { ... }
}
```

**Note**: Check if return type change affects downstream code.

### C3: Hook implementation signature changed

```php
// Before
function mymodule_entity_insert(EntityInterface $entity) { ... }

// After
function mymodule_entity_insert(EntityInterface $entity, $new_param = NULL) { ... }
```

**Note**: Add new parameters with defaults for backward compatibility.

### C4: Configuration schema changed

```yaml
# Before
mymodule.settings:
  type: mapping
  label: 'Settings'
  mapping:
    old_key:
      type: string

# After
mymodule.settings:
  type: mapping
  label: 'Settings'
  mapping:
    new_key:
      type: string
```

**Note**: May need update hook to migrate existing config.

---

## Category D: Architecture Changes — ESCALATE

**Risk**: High — requires understanding of module architecture.
**Patch size**: 20+ lines.
**Auto-patch**: NO — escalate to human.

### D1: Service container restructuring

- Adding/removing services in `.services.yml`
- Changing service class implementations
- Modifying dependency injection

→ **ESCALATE**: Requires understanding of module's service architecture.

### D2: Plugin system changes

- Changing plugin annotations
- Modifying plugin discovery
- Altering plugin manager behavior

→ **ESCALATE**: Plugin changes can break all plugin consumers.

### D3: Event subscriber changes

- Adding/removing event subscribers
- Changing event priorities
- Subscribing to new events

→ **ESCALATE**: Event ordering affects module interaction.

### D4: Database schema changes

- Adding/updating/removing fields in `hook_schema()`
- Writing update hooks for data migration
- Changing entity storage schemas

→ **ESCALATE**: Data loss risk, requires careful testing.

### D5: Routing changes

- Adding/removing routes
- Changing route access controllers
- Modifying route parameters

→ **ESCALATE**: Can break links, forms, and API consumers.

---

## Patch Writing Guidelines

### Patch Format

Always generate patches in unified diff format:

```diff
--- a/path/to/file.php
+++ b/path/to/file.php
@@ -42,7 +42,7 @@
   // Old code context
-  $old_code = deprecated_function();
+  $new_code = replacement_function();
   // More context
```

### Patch Naming

```
<module>-<description>-<drupal-version>-<sequence>.patch

Example: token-fix-deprecated-render-11.x-001.patch
```

### Patch Testing

Before applying a patch:
1. Verify the patch applies cleanly: `git apply --check <patch>`
2. Run `drup validate` after applying
3. Check for regressions in related functionality

### When to Create a New Patch vs. Extend Existing

- **New patch**: Different file, different concern
- **Extend existing**: Same file, related fix
- **Never combine**: Category D changes with A/B/C fixes

---

## Escalation Template

When escalating a Category D issue:

```
Module: <module_name>
Issue: <drupal.org issue URL if exists>
Category: D (Architecture Change)
Description: <what needs to change and why>
Risk: <what could break>
Files affected: <list of files>
Suggested approach: <if known>
```
