---
name: drupal-custom-d11-fixes
description: "Catalog of Drupal 11 deprecation patterns for custom module fixes. Trigger: drup validate finds deprecations in web/modules/custom/ or web/themes/custom/."
version: "1.0.0"
---

# Drupal 11 Custom Code Deprecation Catalog

Use this catalog when `drup validate` or `drup scan` reports deprecations in custom modules or themes. Match the deprecation message to a pattern below and apply the fix.

## How to Use

1. Run `drup scan <path>` or `drup validate <path>`
2. Match the deprecation message to a pattern below
3. Apply the Before → After transformation
4. Re-run `drup validate` to confirm fix

---

## Pattern Catalog

### 1. Container::get() → \Drupal::service()

- **Deprecation**: `Container::get()` is deprecated
- **Complexity**: Low
- **Before**: `$service = \Drupal::getContainer()->get('module_handler');`
- **After**: `$service = \Drupal::service('module_handler');`
- **Edge cases**: None

### 2. file_create_url() → FileUrlGeneratorInterface

- **Deprecation**: `file_create_url()` deprecated in D11
- **Complexity**: Medium
- **Before**: `$url = file_create_url($file->getFileUri());`
- **After**: `$url = \Drupal::service('file_url_generator')->generateString($file->getFileUri());`
- **Edge cases**: Ensure file_url_generator service is available

### 3. file_url_transform_relative() → FileUrlGeneratorInterface

- **Deprecation**: `file_url_transform_relative()` deprecated
- **Complexity**: Low
- **Before**: `$relative = file_url_transform_relative($url);`
- **After**: `$relative = \Drupal::service('file_url_generator')->transformRelative($url);`
- **Edge cases**: None

### 4. file_save_data() → FileStreamInterface

- **Deprecation**: `file_save_data()` deprecated
- **Complexity**: Medium
- **Before**: `$file = file_save_data($data, $uri);`
- **After**: `$file = \Drupal::service('file.repository')->writeData($data, $uri);`
- **Edge cases**: Check destination directory exists

### 5. entity_get_display() → EntityDisplayRepositoryInterface

- **Deprecation**: `entity_get_display()` deprecated
- **Complexity**: Low
- **Before**: `$display = entity_get_display('node', 'article', 'default');`
- **After**: `$display = \Drupal::service('entity_display.repository')->getViewDisplay('node', 'article', 'default');`
- **Edge cases**: None

### 6. entity_get_form_display() → EntityDisplayRepositoryInterface

- **Deprecation**: `entity_get_form_display()` deprecated
- **Complexity**: Low
- **Before**: `$display = entity_get_form_display('node', 'article', 'default');`
- **After**: `$display = \Drupal::service('entity_display.repository')->getFormDisplay('node', 'article', 'default');`
- **Edge cases**: None

### 7. drupal_render() → RendererInterface

- **Deprecation**: `drupal_render()` deprecated
- **Complexity**: Medium
- **Before**: `$output = drupal_render($build);`
- **After**: `$output = \Drupal::service('renderer')->render($build);`
- **Edge cases**: Ensure renderer service available in context

### 8. drupal_render_root() → RendererInterface::renderRoot()

- **Deprecation**: `drupal_render_root()` deprecated
- **Complexity**: Low
- **Before**: `$output = drupal_render_root($build);`
- **After**: `$output = \Drupal::service('renderer')->renderRoot($build);`
- **Edge cases**: None

### 9. drupal_render_placeholder() → RendererInterface

- **Deprecation**: `drupal_render_placeholder()` deprecated
- **Complexity**: Low
- **Before**: `$output = drupal_render_placeholder($build, $args);`
- **After**: `$output = \Drupal::service('renderer')->renderPlaceholder($build, $args);`
- **Edge cases**: None

### 10. format_date() → DateFormatterInterface

- **Deprecation**: `format_date()` deprecated
- **Complexity**: Low
- **Before**: `$date = format_date($timestamp, 'custom', 'Y-m-d');`
- **After**: `$date = \Drupal::service('date.formatter')->format($timestamp, 'custom', 'Y-m-d');`
- **Edge cases**: None

### 11. format_interval() → DateFormatterInterface

- **Deprecation**: `format_interval()` deprecated
- **Complexity**: Low
- **Before**: `$interval = format_interval($timestamp);`
- **After**: `$interval = \Drupal::service('date.formatter')->formatInterval($timestamp);`
- **Edge cases**: None

### 12. format_plural() → TranslationInterface

- **Deprecation**: `format_plural()` deprecated
- **Complexity**: Low
- **Before**: `$text = format_plural($count, '1 item', '@count items');`
- **After**: `$text = \Drupal::translation()->formatPlural($count, '1 item', '@count items');`
- **Edge cases**: None

### 13. entity_load() → EntityTypeManagerInterface

- **Deprecation**: `entity_load()` deprecated
- **Complexity**: Low
- **Before**: `$node = entity_load('node', $nid);`
- **After**: `$node = \Drupal::entityTypeManager()->getStorage('node')->load($nid);`
- **Edge cases**: None

### 14. entity_load_multiple() → EntityTypeManagerInterface

- **Deprecation**: `entity_load_multiple()` deprecated
- **Complexity**: Low
- **Before**: `$nodes = entity_load_multiple('node', $nids);`
- **After**: `$nodes = \Drupal::entityTypeManager()->getStorage('node')->loadMultiple($nids);`
- **Edge cases**: None

### 15. entity_create() → EntityTypeManagerInterface

- **Deprecation**: `entity_create()` deprecated
- **Complexity**: Low
- **Before**: `$node = entity_create('node', ['title' => 'Test']);`
- **After**: `$node = \Drupal::entityTypeManager()->getStorage('node')->create(['title' => 'Test']);`
- **Edge cases**: None

### 16. entity_delete() → EntityTypeManagerInterface

- **Deprecation**: `entity_delete()` deprecated
- **Complexity**: Low
- **Before**: `entity_delete('node', $nid);`
- **After**: `\Drupal::entityTypeManager()->getStorage('node')->delete([\Drupal::entityTypeManager()->getStorage('node')->load($nid)]);`
- **Edge cases**: Load entity first

### 17. watchdog_exception() → LoggerInterface

- **Deprecation**: `watchdog_exception()` deprecated
- **Complexity**: Low
- **Before**: `watchdog_exception('my_module', $exception);`
- **After**: `\Drupal::logger('my_module')->error($exception->getMessage());`
- **Edge cases**: Use appropriate log level

### 18. drupal_set_message() → MessengerInterface

- **Deprecation**: `drupal_set_message()` deprecated
- **Complexity**: Low
- **Before**: `drupal_set_message('Done!', 'status');`
- **After**: `\Drupal::messenger()->addMessage('Done!', 'status');`
- **Edge cases**: None

### 19. file_prepare_directory() → FileSystemInterface

- **Deprecation**: `file_prepare_directory()` deprecated
- **Complexity**: Low
- **Before**: `file_prepare_directory($dir, FILE_CREATE_DIRECTORY);`
- **After**: `\Drupal::service('file_system')->prepareDirectory($dir, FileSystemInterface::CREATE_DIRECTORY);`
- **Edge cases**: Import FileSystemInterface

### 20. file_unmanaged_copy() → FileSystemInterface

- **Deprecation**: `file_unmanaged_copy()` deprecated
- **Complexity**: Low
- **Before**: `file_unmanaged_copy($source, $dest);`
- **After**: `\Drupal::service('file_system')->copy($source, $dest);`
- **Edge cases**: None

### 21. file_unmanaged_move() → FileSystemInterface

- **Deprecation**: `file_unmanaged_move()` deprecated
- **Complexity**: Low
- **Before**: `file_unmanaged_move($source, $dest);`
- **After**: `\Drupal::service('file_system')->move($source, $dest);`
- **Edge cases**: None

### 22. file_unmanaged_delete() → FileSystemInterface

- **Deprecation**: `file_unmanaged_delete()` deprecated
- **Complexity**: Low
- **Before**: `file_unmanaged_delete($path);`
- **After**: `\Drupal::service('file_system')->delete($path);`
- **Edge cases**: None

### 23. file_scan_directory() → FileSystemInterface

- **Deprecation**: `file_scan_directory()` deprecated
- **Complexity**: Medium
- **Before**: `$files = file_scan_directory($dir, '/\.php$/');`
- **After**: `$files = \Drupal::service('file_system')->scanDirectory($dir, '/\.php$/');`
- **Edge cases**: Return type may differ

### 24. drupal_realpath() → FileSystemInterface

- **Deprecation**: `drupal_realpath()` deprecated
- **Complexity**: Low
- **Before**: `$path = drupal_realpath($uri);`
- **After**: `$path = \Drupal::service('file_system')->realpath($uri);`
- **Edge cases**: None

### 25. Unicode::strlen() → mb_strlen()

- **Deprecation**: `Unicode::strlen()` deprecated in favor of native mbstring
- **Complexity**: Low
- **Before**: `$len = Unicode::strlen($text);`
- **After**: `$len = mb_strlen($text);`
- **Edge cases**: Ensure mbstring extension available

### 26. Unicode::strtolower() → mb_strtolower()

- **Deprecation**: `Unicode::strtolower()` deprecated
- **Complexity**: Low
- **Before**: `$lower = Unicode::strtolower($text);`
- **After**: `$lower = mb_strtolower($text);`
- **Edge cases**: None

### 27. Unicode::strtoupper() → mb_strtoupper()

- **Deprecation**: `Unicode::strtoupper()` deprecated
- **Complexity**: Low
- **Before**: `$upper = Unicode::strtoupper($text);`
- **After**: `$upper = mb_strtoupper($text);`
- **Edge cases**: None

### 28. Unicode::substr() → mb_substr()

- **Deprecation**: `Unicode::substr()` deprecated
- **Complexity**: Low
- **Before**: `$sub = Unicode::substr($text, 0, 10);`
- **After**: `$sub = mb_substr($text, 0, 10);`
- **Edge cases**: None

### 29. Drupal\Core\Utility\SafeMarkup::format() → FormattableMarkup

- **Deprecation**: `SafeMarkup::format()` deprecated
- **Complexity**: Low
- **Before**: `$safe = SafeMarkup::format('@name', ['@name' => $name]);`
- **After**: `$safe = new FormattableMarkup('@name', ['@name' => $name]);`
- **Edge cases**: Import FormattableMarkup

### 30. Url::fromUri() → Url::fromRoute() or Url::fromUri()

- **Deprecation**: Some `Url::fromUri()` usages deprecated
- **Complexity**: Medium
- **Before**: `$url = Url::fromUri('internal:/node/1');`
- **After**: `$url = Url::fromRoute('entity.node.canonical', ['node' => 1]);`
- **Edge cases**: Route name must exist

### 31. Config::get() with nested keys

- **Deprecation**: Nested key access patterns changed
- **Complexity**: Medium
- **Before**: `$value = $config->get('nested.deeply.key');`
- **After**: `$value = $config->get('nested')['deeply']['key'] ?? NULL;`
- **Edge cases**: Check array structure

### 32. Database::getConnection() → Connection service

- **Deprecation**: Static database access deprecated
- **Complexity**: Medium
- **Before**: `$conn = Database::getConnection();`
- **After**: `$conn = \Drupal::database();`
- **Edge cases**: None

### 33. db_query() → Connection::query()

- **Deprecation**: `db_query()` deprecated
- **Complexity**: Medium
- **Before**: `$result = db_query('SELECT * FROM {node} WHERE nid = :nid', [':nid' => $nid]);`
- **After**: `$result = \Drupal::database()->query('SELECT * FROM {node} WHERE nid = :nid', [':nid' => $nid]);`
- **Edge cases**: None

### 34. db_select() → Connection::select()

- **Deprecation**: `db_select()` deprecated
- **Complexity**: Medium
- **Before**: `$query = db_select('node', 'n');`
- **After**: `$query = \Drupal::database()->select('node', 'n');`
- **Edge cases**: None

### 35. db_insert() → Connection::insert()

- **Deprecation**: `db_insert()` deprecated
- **Complexity**: Medium
- **Before**: `db_insert('my_table')->fields([...])->execute();`
- **After**: `\Drupal::database()->insert('my_table')->fields([...])->execute();`
- **Edge cases**: None

### 36. db_update() → Connection::update()

- **Deprecation**: `db_update()` deprecated
- **Complexity**: Medium
- **Before**: `db_update('my_table')->fields([...])->condition('id', 1)->execute();`
- **After**: `\Drupal::database()->update('my_table')->fields([...])->condition('id', 1)->execute();`
- **Edge cases**: None

### 37. db_delete() → Connection::delete()

- **Deprecation**: `db_delete()` deprecated
- **Complexity**: Medium
- **Before**: `db_delete('my_table')->condition('id', 1)->execute();`
- **After**: `\Drupal::database()->delete('my_table')->condition('id', 1)->execute();`
- **Edge cases**: None

### 38. db_merge() → Connection::merge()

- **Deprecation**: `db_merge()` deprecated
- **Complexity**: Medium
- **Before**: `db_merge('my_table')->key([...])->fields([...])->execute();`
- **After**: `\Drupal::database()->merge('my_table')->key([...])->fields([...])->execute();`
- **Edge cases**: None

### 39. db_transaction() → Connection::startTransaction()

- **Deprecation**: `db_transaction()` deprecated
- **Complexity**: Low
- **Before**: `$transaction = db_transaction();`
- **After**: `$transaction = \Drupal::database()->startTransaction();`
- **Edge cases**: None

### 40. node_load() → EntityTypeManagerInterface

- **Deprecation**: `node_load()` deprecated (legacy from D7)
- **Complexity**: Low
- **Before**: `$node = node_load($nid);`
- **After**: `$node = \Drupal::entityTypeManager()->getStorage('node')->load($nid);`
- **Edge cases**: None

### 41. user_load() → EntityTypeManagerInterface

- **Deprecation**: `user_load()` deprecated
- **Complexity**: Low
- **Before**: `$user = user_load($uid);`
- **After**: `$user = \Drupal::entityTypeManager()->getStorage('user')->load($uid);`
- **Edge cases**: None

### 42. taxonomy_term_load() → EntityTypeManagerInterface

- **Deprecation**: `taxonomy_term_load()` deprecated
- **Complexity**: Low
- **Before**: `$term = taxonomy_term_load($tid);`
- **After**: `$term = \Drupal::entityTypeManager()->getStorage('taxonomy_term')->load($tid);`
- **Edge cases**: None

### 43. file_load() → EntityTypeManagerInterface

- **Deprecation**: `file_load()` deprecated
- **Complexity**: Low
- **Before**: `$file = file_load($fid);`
- **After**: `$file = \Drupal::entityTypeManager()->getStorage('file')->load($fid);`
- **Edge cases**: None

### 44. hook_init() removed

- **Deprecation**: `hook_init()` removed in D11
- **Complexity**: High
- **Before**: `function mymodule_init() { ... }`
- **After**: Use event subscriber: `KernelEvents::REQUEST` event
- **Edge cases**: Requires service registration

### 45. hook_boot() removed

- **Deprecation**: `hook_boot()` removed in D11
- **Complexity**: High
- **Before**: `function mymodule_boot() { ... }`
- **After**: Use event subscriber: `KernelEvents::REQUEST` with high priority
- **Edge cases**: Requires service registration

### 46. hook_exit() removed

- **Deprecation**: `hook_exit()` removed in D11
- **Complexity**: High
- **Before**: `function mymodule_exit() { ... }`
- **After**: Use event subscriber: `KernelEvents::TERMINATE` event
- **Edge cases**: Requires service registration

### 47. \Drupal::url() → Url object

- **Deprecation**: `\Drupal::url()` deprecated
- **Complexity**: Medium
- **Before**: `$url = \Drupal::url('route.name', ['param' => $value]);`
- **After**: `$url = \Drupal\Core\Url::fromRoute('route.name', ['param' => $value])->toString();`
- **Edge cases**: None

### 48. l() function removed

- **Deprecation**: `l()` removed in D11
- **Complexity**: Medium
- **Before**: `$link = l('Click here', 'route.name');`
- **After**: Use `Link::fromTextAndUrl()` or render array with `#type => 'link'`
- **Edge cases**: Render context matters

### 49. render() function → RendererInterface

- **Deprecation**: `render()` function deprecated
- **Complexity**: Low
- **Before**: `$output = render($build);`
- **After**: `$output = \Drupal::service('renderer')->render($build);`
- **Edge cases**: None

### 50. drupal_get_path() → ExtensionPathResolver

- **Deprecation**: `drupal_get_path()` deprecated
- **Complexity**: Low
- **Before**: `$path = drupal_get_path('module', 'mymodule');`
- **After**: `$path = \Drupal::service('extension.path.resolver')->getPath('module', 'mymodule');`
- **Edge cases**: None

---

## Complexity Guide

| Level | Description | Typical Time |
|-------|-------------|--------------|
| Low | Simple find/replace, single line | < 1 min |
| Medium | May need service injection or import changes | 1-5 min |
| High | Requires architecture change (event subscriber, service registration) | 5-30 min |

## When to Escalate

If a deprecation doesn't match any pattern above, or if the fix requires:
- Database schema changes
- Service container restructuring
- Third-party library API changes

→ Escalate to human review. Do NOT auto-patch.
