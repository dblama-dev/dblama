# Database Workflows Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Complete the approved usability audit and everyday database workflow enhancements.
**Architecture:** Retain existing Vue screens and Go capability APIs. Separate row forms, data transfer, navigation/grid, SQL workspace and database search into independently owned changes, then integrate and verify.
**Tech Stack:** Go, Vue 3, TypeScript, Naive UI, CodeMirror, Vitest, Playwright.
**Spec:** `docs/superpowers/specs/2026-10-29-ux-workflows-design.md`

## Global Constraints

- Preserve Go + Vue 3 + Naive UI + CodeMirror, English and Russian UI, MySQL/MariaDB/PostgreSQL capabilities and single-current-screen navigation.
- No new runtime dependencies, application database, password persistence, remote publishing or new database engines.
- Never run mutations against the user's existing databases; integration and browser tests use disposable containers only.
- Preserve exact DECIMAL/BIGINT values, binary transport, NULL/empty/default distinctions, schema scoping and encoded identifiers.
- PostgreSQL connections remain bound to the login database. MySQL/MariaDB export must honor the selected database under the same account permissions.
- Destructive actions require contextual UI confirmation. Cancellation stops future work and must not claim committed changes were rolled back.
- Public API additions remain backward compatible. Defaults preserve existing dump/import behavior.
- Each parallel owner adds translations in a separate module and merges them into i18n/index.ts only after coordinating; avoid concurrently editing shared locale files.
- Only the coordinator commits while parallel workers are active; each worker lists its owned files in its report. Do not stage others' files, run shared build/E2E or spawn agents.

## Review Focus

- Empty string vs NULL vs server DEFAULT, generated keys and precise numeric values: Task 1 tests payload and actual roundtrip.
- Encoded DB/table names and PostgreSQL schemas retained in all links: Tasks 2/3/5 tests query paths and navigation.
- Browser back, delayed metadata and out-of-order responses never overwrite edited state: Tasks 1/3/4 tests delayed responses and restoration.
- Partial writes/import cancellation never appear as full success: Tasks 1/2 tests stop/error outcomes and missing terminal import record.
- Capability mismatch and corrupt browser storage produce useful fallback instead of crash: Tasks 3/4/5 tests unavailable features and invalid storage.

### Task 1: Reliable row forms, related-record search and bulk editing

**Files:** Modify `web/src/components/RowModal.vue`, `RowModal.test.ts`; create `BulkEdit.vue`, `BulkEdit.test.ts`, focused `web/src/lib/row*.ts` helpers/tests and `web/src/i18n/rowWorkflows.ts`. Only these files owned; TableView integration belongs to Task 3.

**Interfaces:** RowModal retains existing props/events; full metadata must be ready before parent opens it. Optional `loading` prop may protect transitional state. BulkEdit props: `show`, `rowsPath`, `columns:ColumnDef[]`, `rowKeys:Record<string,unknown>[]`; emits `update:show`, `saved`. Existing PATCH row endpoint is used sequentially; parent refreshes after `saved` including partial writes. Local i18n module may be consumed directly using computed locale text helper until coordinator registers it.

- [x] Write failing tests: edit `{name:'',notes:''}` with NOT NULL name saves only changed notes; insert defaults omit generated ID/status/timestamp; explicit empty string and NULL are preserved; untouched DECIMAL/BIGINT/binary remain exact; JSON invalid input gives actionable error; FK search requests a filter and includes selected/manual ID; bulk failure after one success reports one applied and stops.
- [x] Run `npm --prefix web test -- src/components/RowModal.test.ts src/components/BulkEdit.test.ts` (missing new file may initially fail discovery; existing regression must fail behaviorally). Record RED.
- [x] Implement `RowModal` value/NULL/DEFAULT controls and typed editors; reuse small pure payload helpers. Add remote debounced FK filtering with stale response guard and label/key options, referenced-table link and safe manual fallback. Implement confirmed BulkEdit; no new backend transaction claim.
- [x] Run focused tests and frontend full unit suite; report unrelated concurrent failures by name, do not edit others' files.
- [x] Self-review and write task report with changed files/TDD evidence. Coordinator commits after integration.

### Task 2: Database-aware export and usable CSV import

**Files:** Own `server/internal/api/handlers_transfer*`, `server/internal/driver/types.go` (ExportRequest/CSVImportOpts only), MySQL/Postgres dump/import implementations and transfer-specific tests; `web/src/views/ExportView*`, `ImportView*`, `web/src/lib/importFile*`, new CSV preview helpers/tests, `web/src/api/types.ts` (ExportRequest only), `web/src/i18n/transferWorkflows.ts`. Discover exact driver export/import files with rg before edits. Do not touch router.go or common locale files.

**Interfaces:** ExportRequest adds optional `mode:'all'|'structure'|'data'`. CSV mapping adds multipart `columns` JSON array of target column names in source-field order; empty target means ignored source column, duplicate/nonexistent names rejected. Mapping omitted preserves current positional behavior. `runImport(db,form,onProgress,schema?,signal?:AbortSignal)` requires terminal success. `table` route query preselects import/export.

- [x] Add failing Go tests for export of other MySQL database, PostgreSQL mismatch, all/structure/data SQL content and invalid mode; CSV mapped/reordered/ignored columns, unknown/duplicate mapping and quoted Unicode rows. Add frontend tests for CSV quoted comma/newline parser, preselection, abort and premature EOF rejection.
- [x] Run focused Go/frontend tests and record RED before implementation.
- [x] Implement selected-database routing with driver-safe qualification/connection scope. Default mode is all; data skips DDL and views; structure skips data. Validate mapping server-side, preserve schema and exact values. Add bounded preview and mapping UI with CSV presets, cancellation and partial-work explanation.
- [x] Run Go unit suite and frontend tests, plus targeted disposable DB integration tests for changed import/export behavior (coordinate build/E2E with root). Record GREEN.
- [x] Self-review and report changed files and residual limits. Coordinator commits.

### Task 3: Navigation, grid state, compact structure and existing operations

**Files:** Own `web/src/views/TableView*`, `DbView*`, `views/structure/StructureEditor*`, `components/DataGrid*`, `ObjectTree*`, `AppSidebar.vue`, `AdvancedNavigation.vue`, `layouts/BasicLayout.vue`, `components/TableOperations.vue` and tests, `web/src/lib/tableQuery*`, navigation/preference helpers/tests, `web/src/i18n/navigationWorkflows.ts`. Do not change RowModal/BulkEdit or ConsoleView/ImportView/ExportView or router/index.ts.

**Interfaces:** Export/import routes use query `table`; console query `table` identifies context. Query keys `filters` (JSON RowsFilter[]), `sort` (JSON RowsSort), `page`, `pageSize` retain state; preserve unrelated query keys incl schema/view. DataGrid takes controlled `filters` and emits existing apply-filters. Add `edit-rows` carrying captured row keys; parent renders Task 1 BulkEdit. TableOperations uses existing truncate/drop endpoints. New DB search link `/db/:db/search` implemented by Task 5.

- [x] Add failing behavior tests for restored visible filter after tab switch, parsed route query/back, schema-preserving context links, default metadata loaded before opening insert, confirm-before-single-delete, select-all bulk table operations and capability-gated partitions. Test corrupt preference storage fallback and selection derived from route.
- [x] Run focused component/helper tests and record RED.
- [x] Implement route-backed query state; await full definition before opening row forms; add BulkEdit integration without changing its file. Make grid cells bounded/full-inspectable with useful widths/local scroll, sticky controls and pagination/count top/bottom. Provide accessible labels and Enter-to-filter.
- [x] Implement context actions and clickable breadcrumbs, database list filtering/sorting/badges/quick actions, single/bulk truncate/drop with confirmation; make Sidebar collapse/resize and recent/favorites safe local preferences. Keep global links above long trees. Compact structure overview with expandable edits, guard dirty changes and independent save scopes without wiping other pending edits.
- [x] Run focused and full frontend tests/type checking; report cross-task missing components while work is in progress. Document UI choices and any outstanding integration needs.
- [x] Self-review and report; coordinator commits.

### Task 4: SQL drafts, bookmarks, completion and common SQL editing

**Files:** Own `web/src/views/ConsoleView.vue`, `web/src/lib/useConsole*`, new SQL persistence/editor helpers and tests, `web/src/components/SqlEditor.vue`, `web/src/views/ObjectsView*`, `web/src/views/UsersView.vue`, `web/src/i18n/sqlWorkflows.ts`. Root registers all translation modules in `web/src/i18n/index.ts` once parallel modules exist.

**Interfaces:** SQL storage scope includes driver flavor/host/user/database/schema; connection host should include available port. Persistent local drafts load by scope, save on input, and handle storage exceptions. Console `table` query provides quoted starter SELECT only with no existing draft and never executes. Shared SqlEditor props `modelValue`, `dialect`, `schema?`; emits `update:modelValue`, `execute` with selected SQL or undefined. Object editors use the same editor with dirty navigation guard.

- [x] Add tests for draft isolation/restoration/storage failure, bookmark save/replace/remove, keyboard selection execution and no overwrite after delayed completion metadata. RED verified for storage/scope/guards; the initial shared-editor RED was not established (see verification note).
- [x] Run focused tests and record RED.
- [x] Implement bounded drafts/bookmarks and dialect-aware schema completion; integrate existing history, formatting, explain/cancel and Ctrl/Cmd+Enter. Add reusable SqlEditor to object sources and preserve accessible test locators where possible. Unify control styling in touched administrative screens.
- [x] Run focused/full frontend tests and type checking; commit this task's owned files after other workers are ready or by exact paths.

### Task 5: Bounded database-wide text search

**Files:** Create `web/src/views/SearchView.vue`, `SearchView.test.ts`, `web/src/lib/databaseSearch.ts`, `databaseSearch.test.ts`, `web/src/i18n/searchWorkflows.ts`; modify `web/src/router/index.ts` and register translation module.

**Interfaces:** Uses existing catalog/table definition/rows endpoints with encoded paths and catalog.schema. `searchDatabase` accepts selected tables and search text, onResult callback and AbortSignal; concurrency 2, max 20 selected tables per run, max 20 matches per text-column query, request timeout/stop bounded. Result includes table,column,rows and filter link using Task 3 query `filters` JSON. Literal `%`, `_`, and backslash escaped for LIKE. Reject empty query and clearly show per-column limits.

- [x] Write failing tests: names containing spaces/%/slash keep correct URLs; literal search escapes metacharacters; selecting 21 tables is rejected; cancellation prevents subsequent work; one inaccessible table reports an error while others continue; result link preserves schema/filter.
- [x] Run `npm --prefix web test -- src/lib/databaseSearch.test.ts src/views/SearchView.test.ts` and record RED.
- [x] Implement bounded sequential-per-worker text searches with existing parameterized API and cancellation. Render selectable tables, visible scope/limit explanation, progress, errors, matching rows and table links. Restrict to supported text columns, never execute arbitrary generated SQL.
- [x] Run focused tests/type checking and commit owned files.

### Task 6: Integration, browser regressions, review and delivery

**Files:** `web/e2e/ux.spec.ts`, shared fixtures only as needed, README/CHANGELOG and this plan, `docs/superpowers/reports/2026-10-29-ux-workflows.md`; no changes to existing test expectations merely to hide regressions.

- [x] Integrate all translation modules and interfaces; add E2E regressions for A1–A6, default row insertion, bulk edit, table operations, SQL draft/bookmark and transfer modes/mapping. Existing behavior must continue to work on all three engines.
- [x] Run `make backend-test web-test lint`, `make backend-integration`, `make build`; fix actual failures, then run `scripts/check-e2e.sh mysql`, `mariadb`, `postgres` sequentially.
- [x] Conduct browser inspection using disposable database only: grid long TEXT at 1366×768 and narrow window, compact structure, table navigation, modal types and SQL editor. Save before/after screenshots to report directory, stop temporary resources.
- [x] Obtain fresh task reviews and whole-branch review per superpowers; fix Important/Critical issues and rerun affected validation. Record minor residual limitations explicitly.
- [x] Update implementation report, check every spec outcome against changes/tests, commit changes and retain local reviewable branch. Do not push/publish. Return concrete results, verified test counts and artifact paths.

Verification note: Task 4 observed RED for storage, stale scope and object guards. The initial shared editor run overlapped implementation; it is not claimed as a verified RED run. Later selection/theme regressions and review fixes have separate recorded evidence.
