# Task 3 — Navigation, grid state, structure and operations

Implemented in the shared `ux-workflows` worktree; no staging, commits, production database mutations, shared build or E2E runs.

## Changes

- Grid state is serialized in route `filters`, `sort`, `page`, `pageSize`; browser history restores visible controls and the actual server query. Unrelated query keys, view and schema survive updates. Invalid route state falls back safely; CONTAINS search links are accepted.
- Separate primary-key and row request sequences prevent a pending PK request from being discarded by a filter request. Returning from structure to data refreshes both metadata and rows. Route acknowledgements do not issue duplicate row queries.
- Full table definition must load successfully before insert/edit/copy/bulk forms open. BulkEdit uses the agreed props/events; its `saved` event refetches without closing the modal, preserving partial-write reports. Selected identities are captured before awaiting metadata.
- Single row deletion now requires confirmation with table and row identity; bulk confirmation includes table and count. Selection clears on result replacement so keyless positional keys cannot silently refer to different rows. Long cell values use bounded expandable previews; widths, horizontal table scrolling, fixed row actions and top/bottom pagination/counts are supplied. Filter controls have labels and Enter applies filters.
- Server/database/schema breadcrumbs and table actions expose Data, Structure, CREATE SQL, Insert, SQL query, Search, Import, Export, Operations and capability-gated Partitions. Table context and schema survive links; partitions use the existing table-specific route.
- TableOperations confirms truncate/drop using existing single-table APIs, including `isView=true` when dropping a view; truncation of views is blocked. Database list has filtering, sorting, view badges, estimated row counts, quick actions, select-all of visible base tables and confirmed bulk truncate/drop. Existing capability-gated copy/move receives the selected base tables.
- Sidebar global links precede the tree. Collapse and a bounded width slider persist locally; narrow screens start collapsed with an accessible reopen control. Corrupt/unavailable storage is tolerated. Recent/favorite tables are bounded and scoped to driver, host, user, database and schema. Selection follows route params/history. Database/schema selector mutations occur only after successful navigation so cancelled dirty-editor navigation leaves scope unchanged.
- Existing columns start as compact native details/summary rows, while new/create columns remain expanded. Columns/options, indexes and FK have separate save scopes. Index/FK saves update only their baselines/catalog; column reloads retain pending index/FK models. Dirty state guards navigation and browser unload; table rename warns before discarding other pending scopes. Initial load does not warn.

## Owned files

- `web/src/views/TableView.vue`, `TableView.test.ts`, `DbView.vue`, `DbView.test.ts`
- `web/src/views/structure/StructureEditor.vue`, `StructureEditor.test.ts`, adjacent approved `IndexEditor.vue`
- `web/src/components/DataGrid.vue`, `DataGrid.test.ts`, `TableOperations.vue`, `TableOperations.test.ts`, `ObjectTree.vue`, `ObjectTree.test.ts`, `AppSidebar.vue`, `AdvancedNavigation.vue`
- `web/src/layouts/BasicLayout.vue`, `BasicLayout.test.ts`
- `web/src/lib/tableQuery.ts`, `tableQuery.test.ts`, `navigationPreferences.ts`, `navigationPreferences.test.ts`
- `web/src/i18n/navigationWorkflows.ts` exports `navigationWorkflowMessages`; coordinator registered it.

## TDD and debugging evidence

Observed expected RED failures before implementation for restored route/visible filters, delayed defaults, single-row confirmation, route query helpers, safe preferences, select-all operations, compact overview and preserving pending columns after index save. Additional RED/GREEN cycles cover route-driven selection, duplicate fetches, delayed PK metadata, positional selection replacement, cancelled database/schema navigation, narrow sidebar, returning to data after structure, cancelled/duplicate insert navigation, CONTAINS restoration, exact partitions context and view-aware drop API.

Representative initial run: 7 expected behavior failures / 34 passes in tableQuery, TableView and DataGrid. First broad focused verification: 8 files / 77 passing tests; expanded focused verification: 9 files / 89 passing tests before final integration cases. Latest full frontend suite at 11:40 UTC: 43 files / 273 passing tests. Final focused/type/lint verification is recorded below when completed.

Earlier concurrent whole-suite failures were reported to coordinator: Task2 CSV preview width bound and ImportView WrapperLike test types; Task1 ParentOption and Vue filter parser issues; Task5 SearchView URL optional-string test type. These were external to this task and subsequent checks supersede their status.

## Integration and limits

- Coordinator/Task5 must defer catalog schema mutation until successful navigation. This was explicitly coordinated after discovering that early router mutation could run before dirty guards.
- Existing browser structure-edit workflows must expand a saved column's summary before filling its controls. This is intentional compact-overview behavior, not a removed editor.
- Browser visual QA, production build, disposable-engine integration and E2E matrix remain coordinator-owned. Unit/component tests verify functionality but do not replace laptop/narrow layout inspection.
- Sidebar resize is an accessible bounded slider rather than a drag handle. No pointer listeners are installed. Preference storage is optional and failures do not block database workflows.

## Final verification

- Focused command across nine owned test files: **92/92 tests pass**, zero failures (11:41 UTC).
- `npm --prefix web run type-check`: **pass** (Vue and Node TS projects).
- `npm --prefix web run lint`: **pass**, zero errors/warnings.
- `git diff --check`: **pass**.
- Latest complete `npm --prefix web test`: **43 files / 273 tests pass**; the final view-aware drop endpoint adjustment was subsequently reverified in the 92-test focused run above. JSDOM printed the existing non-failing “navigation to another Document” notices.

## Follow-up — cell accessibility and bounded rendering

Browser E2E found that wrapping every value in closed `details` removed numeric cells' accessible names. Hidden full-value `pre` nodes also duplicated cell text. The original scoped CSS did not reach `h`-rendered cell nodes, allowing long values to expand rows.

- Added regressions first: exact single-copy BIGINT/DECIMAL cell text/names and bounded long-value preview/full inspector both failed as expected. Added a practical `getComputedStyle` test on actual rendered nodes; it failed with `white-space: normal`.
- Short values now render as plain spans with explicit cell names. Long values have a one-line, 120-character preview and named Preview button opening the exact value in a modal outside the table. Cells contain no hidden duplicate full text.
- Compact nowrap/ellipsis/overflow bounds are inline on rendered cell nodes, so they apply independently of scoped CSS. The full inspector is scrollable and bounded to 70vh. Decorative button CSS uses `:deep`.
- Final focused DataGrid verification: **25/25 tests pass** (11:50 UTC). Typecheck, lint and diff whitespace checks pass. The viewport-bound test assertion was corrected to compare computed pixels against 70% of viewport height.
- Full frontend run started concurrently and contains external in-progress failures in `download.test.ts` (three reversible CSV export tests) and `importFile.test.ts` (literal CSV encoding), plus the superseded viewport-unit assertion imported before correction. Do not treat that concurrent run as a final whole-suite success; coordinator owns final stable verification. No build, E2E, staging or commits performed in this follow-up.

## Review fix round 1

Addressed all three Important findings from `task-3-review.md` plus the requested scroll/port improvements.

1. **Every value can be inspected:** a native keyboard-operable Preview action per row opens the exact-value modal with a column picker. This covers clipped URLs/identifiers shorter than 121 characters while preserving plain numeric cell text, exact accessible cell names and no controls on each tiny ID. Long cells retain their direct Preview action. No hidden full-value copy is added to cells.
2. **Refresh clean dependent metadata:** after column/options save, clean index/FK scopes and their baselines refresh from the authoritative returned definition. Dirty index/FK scopes remain untouched. Regression renames an indexed column, checks the refreshed index/FK references, and confirms no false unload warning; existing pending-scope preservation regression remains green.
3. **Do not mark later index edits saved:** IndexEditor captures the submitted model before POST and includes it in the saved event. StructureEditor records that snapshot as the saved baseline. Both an isolated delayed-request child test and an actual parent POST/edit/navigation sequence prove that a newer index edit remains visible and dirty after the earlier request succeeds.
4. **Reachable horizontal scrollbar:** NDataTable's internal body uses `max(240px, calc(100vh - 360px))`, keeping local vertical scrolling and the horizontal scrollbar near the viewport rather than below all 50 rows.
5. **Connection-specific preferences:** recent/favorite identity includes the separate connection port. A component regression switches 3306 → 3307 → 3306 and verifies favorite isolation/restoration.

TDD: six expected failures observed before fixes (full short-value inspection, viewport body bound, port scope, submitted index snapshot, stale clean index/FK metadata, and wrong dirty baseline). Final verification: **4 focused files / 66 tests pass**, Vue/Node **typecheck passes**, **lint passes**, **git diff --check passes** (12:04 UTC). Added `web/src/views/structure/IndexEditor.test.ts`; other changes stay in the previously owned grid/tree/structure files and this report. No full-suite rerun, shared build, E2E, commits, staging or subagents in this round; coordinator owns stable final/browser verification.
