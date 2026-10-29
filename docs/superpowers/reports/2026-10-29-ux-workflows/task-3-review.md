### Spec Compliance

- ❌ Issues found: full inspection is unavailable for values that overflow a cell but do not meet the character/newline threshold (`web/src/components/DataGrid.vue:205`, `:222`). Independent save scopes preserve pending models, but also preserve stale clean metadata and can mark edits made during an index save as saved (`web/src/views/structure/StructureEditor.vue:138`, `:196`).
- ✅ Route query restoration and unrelated-key preservation are implemented in `web/src/lib/tableQuery.ts:70` and `web/src/views/TableView.vue:151`; controlled filter restoration is covered at `web/src/components/DataGrid.test.ts:382`. Single-row and bulk destructive actions have contextual confirmation in `web/src/components/DataGrid.vue:280`, `web/src/components/TableOperations.vue:5`, and `web/src/views/DbView.vue:17`. Capability-gated partitions and encoded context links appear in `web/src/views/TableView.vue:17`.
- ⚠️ Cannot verify from this task diff: final whole-branch English/Russian registration, the router's successful-navigation schema synchronization, all-engine integration, and final stable full-suite verification. The controller supplied a MySQL browser 16/16 result; the task report explicitly leaves final matrix verification to the controller.

### Strengths

- `web/src/views/TableView.vue:185` separates primary-key and row request generations, preventing filter requests from invalidating delayed primary-key metadata. Definition loading at `:309` gates form opening on successful full metadata, with stale-response checks.
- `web/src/components/DataGrid.vue:300` clears selection when rows are replaced, preventing positional keys from selecting different keyless rows. `web/src/views/TableView.vue:344` captures bulk identities before waiting for metadata.
- `web/src/components/ObjectTree.vue:91` and `:193` navigate before changing shared scope, so cancelled dirty-editor navigation leaves the database/schema unchanged. `web/src/lib/navigationPreferences.ts:7` tolerates corrupt or unavailable storage and bounds persisted collections and width.
- `web/src/views/structure/StructureEditor.vue:462` provides a compact native details/summary overview; separate baselines and navigation/unload guards at `:123` protect ordinary unsaved section edits.

### Issues

#### Critical (Must Fix)

- None.

#### Important (Should Fix)

- **Overflowed values shorter than 121 characters cannot be fully inspected.** `web/src/components/DataGrid.vue:205` only enables the inspector above 120 characters or three newlines. The other branch at `:222` applies nowrap/ellipsis/hidden overflow without a title or inspector action, while columns can be just 140px wide (`:256`). A 60-character URL, identifier, or two-line value is clipped with no ordinary mouse/keyboard full-value view; an aria-label does not provide a visible inspector. This misses the bounded/full-inspectable requirement. Make inspection available for every potentially clipped value, or detect actual overflow and expose the same exact-value modal. Cover an overflowing value below the current threshold.

- **Saving columns leaves clean index/FK metadata stale.** `web/src/views/structure/StructureEditor.vue:420` always reloads with `columnsOnly=true`; `:196` then ignores the authoritative returned indexes and foreign keys even when those sections have no pending edits. Renaming an indexed column changes the server's index column references; dropping an indexed column can remove its index. The UI retains the old index, and its clean baseline remains unchanged. Subsequent index editing/saving operates against names that no longer exist. Preserve only dirty scopes, refreshing clean scopes and their baselines from the returned definition. Add a column-rename/drop case alongside the existing pending-edit preservation test.

- **Index edits made while save is in flight are incorrectly marked clean.** `web/src/views/structure/StructureEditor.vue:138` snapshots the current index model after the child request completes, rather than the model submitted to that request. The existing child keeps its ordinary index inputs editable while awaiting the POST (`web/src/views/structure/IndexEditor.vue:121`, `:157`). If index A is submitted and the user changes it to B before completion, A is persisted but B becomes the clean baseline; navigation silently discards B and the guard does not warn. Disable index editing while saving, or pass the submitted snapshot in the saved event and baseline that exact snapshot while keeping subsequent changes dirty. Cover the delayed-save/edit/navigation sequence.

#### Minor (Nice to Have)

- `web/src/components/DataGrid.vue:408`: table-local horizontal scrolling exists, so the reported scrollbar position does not by itself violate the local-scroll requirement. However, without a vertical bound or synchronized sticky horizontal control, the scrollbar is below all 50 rows; inspecting rightmost columns requires scrolling to the bottom and back. A viewport-bounded table body or accessible sticky horizontal scrollbar would improve the required laptop usability.
- `web/src/components/ObjectTree.vue:127`: preference scope omits the separate connection port (`web/src/api/types.ts:16`), so two servers on the same host with the same flavor/user share recent/favorite table locations. Include port in connection identity; this is lower impact than SQL draft leakage because these preferences contain navigation locations only.
- `.superpowers/sdd/2026-10-29-ux-workflows/task-3-report.md`, Final verification: reported JSDOM “navigation to another Document” notices remain noisy despite passing tests. Mock the relevant browser navigation boundary so successful validation is pristine. Final full-suite success must come from the controller's stable run, since the report correctly documents a concurrent run with failures.

### Assessment

**Task quality:** Needs fixes.

**Reasoning:** The main navigation, confirmation, metadata-gating, and compact overview work is coherent and covered by useful behavior tests. Three concrete gaps remain in full-value inspection and preserving an accurate saved/unsaved structure state.

**Checks:** Read the task brief, report, design, and complete supplied diff; no tests rerun and no code/index/branch changes made. Focused outside-diff checks: inspected `IndexEditor.vue`'s full watcher/save/input contract because the changed emission hunk cuts off that save flow and the new parent baseline depends on it; inspected `ForeignKeyEditor.vue`'s disabled-while-saving contract for the same risk; inspected `catalog.ts` to establish that invalidation refreshes only the table list, not index/FK definitions; inspected `TableTransfer.vue` for capability gating and `api/types.ts` for connection identity/port fields. One attempted backend path search returned nonexistent paths; no backend claim relies on it. Only this requested review artifact was written.
