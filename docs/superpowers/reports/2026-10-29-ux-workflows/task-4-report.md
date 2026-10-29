# Task 4 — SQL workspace and editing consistency

Implemented shared CodeMirror SQL editor, dialect/table/column suggestions, Ctrl/Cmd+Enter execution, per-connection/user/database/schema drafts and named saved queries. Storage is bounded and failures visible without breaking editing. Metadata refresh reconfigures completion without replacing text. Connection identity includes port. Query completion from a previous scope cannot replace current results. Query history storage failures no longer turn successful SQL into an error.

Object source now uses the same editor and Naive buttons. Object editing and structure editing guard dirty navigation; shared useDirtyGuard covers browser unload/router navigation. Users view buttons use Naive controls.

Owned modules: SqlEditor.vue/test, sqlWorkspace.ts/test, sqlWorkflows.ts, useConsole.ts/test, consoleHistory.ts, useDirtyGuard.ts, ConsoleView.vue, ObjectsView.vue/test, UsersView.vue. Coordinator registers all translation modules in i18n/index.ts with safe locale storage.

TDD: observed RED for draft restore/storage failures (/tmp/dbm-sql-red.log), stale response after database switch (/tmp/dbm-console-scope-red.log), object switch cancellation (/tmp/dbm-objects-red.log). Shared editor tests cover text preservation after metadata and CtrlEnter; initial run overlapped implementation and does not constitute verified RED.

Fresh focused validation: 5 files/31 tests passed (sqlWorkspace, useConsole, SqlEditor, ObjectsView, UsersView), /tmp/dbm-task4-final.log. Whole frontend type/lint previously passed after owner corrections; final coordinator pass and E2E pending.

Limits: drafts/bookmarks remain local to browser, capped30workspaces/50queries/1MB each. Column suggestions load for selected table. No cross-device sync or automatic SQL execution.

## Review fix round 1

Resolved the two important review findings and the selection-test gap:

- Account creation and the rename/password/limit Save buttons now have `attr-type="submit"`. Regressions click the actual rendered Naive buttons mounted in the document and assert the corresponding POST/PATCH/PUT request, including exact submitted values and password clearing.
- Partition SQL now participates in the shared dirty predicate against the cleared-source baseline. Refresh, partition-table changes, router navigation and destructive operations that reload source all check for discard first. Cancelling restores the selected table and preserves source without fetching or mutating. Tests also cover browser unload, an initially clean baseline, accepted discard and the clean baseline after a successful save.
- SQL editor keyboard coverage now dispatches a real CodeMirror selection spanning `SELECT 2;` within a two-statement document, sends Ctrl+Enter and verifies that only the selected SQL is emitted without changing the document. The old no-selection regression remains with an accurate name.

TDD evidence: `/tmp/dbm-task4-fix-red.log` records eight expected failures before the production fix: four missing click-to-API submissions and four missing partition discard checks. `/tmp/dbm-task4-fix-focused.log` then records 3 files / 23 passing tests. ObjectsView tests provide jsdom's missing Range layout metrics and complete locale messages so these covering tests emit no application warnings.

Fresh whole-frontend type checking and lint both passed (`/tmp/dbm-task4-fix-types.log`, `/tmp/dbm-task4-fix-lint.log`); owned-file `git diff --check` passed. Full frontend unit result is recorded below when the run finishes. Build and browser E2E remain the coordinator's responsibility for this round, per delegated scope.

Full frontend unit suite passed: 44 files / 297 tests, exit 0, `/tmp/dbm-task4-fix-full.log` (45.59s). Only Vitest's environment-performance advisory remains; no failed tests or application warnings were reported in this run.
