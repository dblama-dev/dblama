### Spec Compliance

- ❌ Issues found: partition source editing is excluded from the new dirty guard (`web/src/views/ObjectsView.vue:55`), so dirty object editing can still lose work without warning.
- ❌ Existing behavior regressed during the requested control unification: account creation, rename, password change and limit saving no longer submit when their buttons are clicked (`web/src/views/UsersView.vue:10`, `:16`, `:17`, `:18`).
- ⚠️ Cannot verify from this diff: structure-editor dirty protection is not changed here. Controller should verify the structure worker's implementation; `web/src/lib/useDirtyGuard.ts:4` supplies the reusable guard but does not demonstrate structure integration.

### Strengths

- `web/src/lib/sqlWorkspace.ts:7` uses a collision-safe serialized identity including flavor, host, port, user, database and schema; `:10` validates corrupt storage and `:21` bounds persisted workspaces/bookmarks.
- `web/src/components/SqlEditor.vue:36` reconfigures only the language compartment when metadata changes, preserving edited text. `web/src/views/ConsoleView.vue:65` requests columns independently of SQL editing.
- `web/src/lib/useConsole.ts:78` rejects stale execution responses after scope changes, and `web/src/lib/consoleHistory.ts:33` prevents history-storage failures from changing the outcome of successful SQL execution.

### Issues

#### Critical (Must Fix)

- None.

#### Important (Should Fix)

- `web/src/views/UsersView.vue:10`, `:16`, `:17`, `:18`: native submit buttons were replaced with `NButton` without `attr-type="submit"`. Naive UI defaults its underlying button to `type="button"`, so clicking Create or the three Save buttons no longer invokes the form submit handlers. Add `attr-type="submit"` to each form submit control and a regression test that clicks the rendered button rather than directly submitting the form.
- `web/src/views/ObjectsView.vue:55`: dirty detection requires `editing.value`, which is only enabled for ordinary object selection/creation. The partition SQL textarea (`:10`) never enables it. Typing an ADD PARTITION clause and refreshing, selecting another partition table, or leaving the screen therefore clears the source without confirmation. Include partition-source changes in the dirty predicate and guard the table switch before its reload; cover cancellation of refresh/table/navigation with a partition draft.

#### Minor (Nice to Have)

- `web/src/components/SqlEditor.test.ts:13`: the test named “runs selected SQL” never creates a selection and only asserts `execute(undefined)`. Add an actual document selection and assert the emitted SQL substring; the task explicitly requested keyboard selection execution coverage.

### Assessment

- **Task quality: Needs fixes.** Persistence and editor reuse are sound, but administrative submit actions are broken and one SQL editing surface still discards pending input without warning.
- **Checks:** reviewed the supplied diff; no tests rerun. Read `/tmp/dbm-task4-final.log`: five files/31 tests passed, with a Vitest environment-performance advisory. The report explicitly leaves final coordinator type/lint/E2E validation pending.
- **Focused outside-diff check — NButton form semantics:** `web/node_modules/naive-ui/es/button/src/Button.mjs:63` defaults `attrType` to `button`, and `:442` applies it to the native element.
- **Focused outside-diff check — schema changes bypassing route guards:** `web/src/components/ObjectTree.vue:95` navigates before changing shared schema and returns on navigation failure; `web/src/stores/catalog.ts:58` then updates scope. No schema-switch bypass finding.
- **Cut-off hunk check:** read the remainder of `web/src/views/ObjectsView.vue:68` onward because the diff ends mid-function. `:110` reloads on partition-table/schema changes; this confirms the partition draft loss above.
