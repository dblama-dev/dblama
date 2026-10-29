# Final review fix wave

Status: all four Important findings in `final-review.md` addressed, plus the coordinator's dark SQL editor readability finding. Changes are in the shared `/home/hitslab/.codex/worktrees/ux-workflows/db-manager` worktree. No staging, commits, backend edits, build, E2E execution, full-suite repetition, or subagents by this worker. Coordinator owns the shared build, browser matrix, visual confirmation, and already-running backend integration.

Read the whole final review, approved `docs/superpowers/specs/2026-10-29-ux-workflows-design.md`, relevant Task 1/3/5 reports and existing sources/tests. Applied systematic debugging, test-driven development (including writing-good-tests guidance), and verification-before-completion.

## Changes and contract preservation

### 1. P1: selected duplicate keyless rows

The drivers' existing PATCH predicates update every matching row. A keyless row snapshot therefore cannot identify one selected duplicate safely. The new bulk action now conservatively requires declared primary-key metadata and a complete, non-null primary-key value for every selected row. DataGrid disables the action and explains the primary-key requirement in English/Russian. Its handler also refuses to emit the action when identities are incomplete. TableView independently rejects missing/incomplete PK identities before loading definitions or opening BulkEdit; it captures only primary-key fields.

This changes only the new bulk-edit affordance. Existing row Edit/Delete/Copy behavior remains outside this finding. BulkEdit's agreed props/events, sequential execution, confirmation, partial-write reporting and refresh behavior remain unchanged.

Regressions select only one of two identical keyless rows and verify no bulk event, disabled action and explanatory text; verify normal composite-PK selection still emits the captured identities; and directly exercise TableView rejection of keyless/incomplete events. The real-browser case creates two identical rows in a disposable fixture, selects one, verifies the disabled operation/explanation, reloads and verifies both values remain unchanged.

### 2. P2: MySQL/MariaDB TIME durations

RowModal now uses the native clock control only for a known PostgreSQL connection. MySQL, MariaDB and unknown engines retain text input for TIME, including TIME precision declarations. Original field snapshots and payload comparison are unchanged, so an unrelated edit omits untouched durations. No Number/Date conversion or truncation is introduced.

Behavioral regressions cover both MySQL/MariaDB flavors, `36:00:00.123456` and `-01:30:00.000001` display, unrelated PATCH omission, editing `-36:10:11.123456`, and inserting `48:01:02.123456`. Existing PostgreSQL date/native-clock/timestamp tests remain passing. The real-browser duration case exercises actual row endpoints on MySQL/MariaDB and skips PostgreSQL with an explicit engine explanation.

The existing NULL-toggle test's stale expectation for visible Russian “обязательно” was corrected to assert the actual NOT NULL tooltip. The coordinator confirmed its earlier cosmetic change intentionally renders NOT NULL instead; this failure existed in the first RED run and required no production behavior change.

### 3. P2: direct-loaded view operations with a collapsed sidebar

TableOperations now resolves authoritative object kind from its own scoped catalog GET independently of ObjectTree/AppSidebar. Both destructive controls stay disabled when metadata is pending, absent, malformed or failed. Views never permit truncation; confirmed view deletion retains the existing `/tables/{table}?isView=true` contract.

The workflow captures database/table/schema path and a generation. Scope changes reset kind/busy/error, discard old kind responses and prevent late mutation completion from invalidating/navigating the new scope. An additional resolved-path guard prevents using an older scope's kind before the next Vue watcher flush. Unmount also invalidates pending generations. Confirmations and catalog invalidation remain contextual and schema-aware.

Regressions mount TableOperations with no sidebar and empty catalog, cover scoped/encoded table and view operations, pending and unknown-kind action guards, catalog failure, old-schema response arrival and late deletion completion after navigating to a different database/table. The real-browser regression direct-loads view operations at 680px (sidebar unmounted), verifies truncate disabled, confirms drop, checks `isView=true` and success, and verifies the actual catalog no longer contains the view.

### 4. P2: Search URL context versus competing catalog loaders

The catalog store now records `tablesPath` only alongside a successful accepted table response. Existing request-ID response-order guards remain intact. Search keeps one pending table-context initialization tied to the requested database/schema path and initial selection identity. It applies context from the next successful current-scope catalog, including a newer same-scope loader, then consumes the initialization. Different scopes, failures and user changes cannot apply stale initialization or reset manual selections.

The new controlled component regression starts Search's catalog request, starts a competing same-scope store request, resolves Search's older request first, then resolves the current request. The target is correctly selected; manual deselection plus selection of a different table survive later refresh. Existing encoded-context, unknown-context, manual-selection and stale-database tests pass.

The new browser test controls ordering of actual `/api/databases` and table-catalog responses with Playwright `route.fetch`/`fulfill` gates. It mounts the real expanded sidebar and search page, makes Search's request finish after a newer request has started, then checks URL preselection and manual selections surviving a sidebar remount/refetch. Responses come from the real backend; no fabricated database response is supplied.

### 5. Coordinator follow-up: dark SQL editor readability

SqlEditor now derives CodeMirror appearance from Naive UI's active theme variables. It provides matching editor/gutter backgrounds and text, cursor, active-line, selection and tooltip styles, plus readable SQL keyword/string/number/comment/type colors for light/dark themes. This uses already-installed CodeMirror/Lezer and Naive UI modules; package manifests/lockfiles and runtime dependency versions are unchanged.

A dedicated CodeMirror Compartment is reconfigured when the theme changes. The EditorView, document, selection, undo history and existing language/editability/keybindings are retained. The focused regression switches light → dark → light, verifies the same EditorView/document/selection, checks keyword and gutter text contrast at least 4.5:1, and checks gutter/background luminance remains consistent. The test also ensures theme changes emit no document updates. Coordinator owns before/after screenshots and real visual confirmation.

## RED/GREEN and verification evidence

1. `/tmp/final-fix-red.log`: first focused pre-fix run across DataGrid, RowModal, TableOperations, TableView and SearchView produced 12 failed / 83 passed tests. All four review findings reproduced. One was the pre-existing required-label assertion noted above. Initial TableOperations test setup also accidentally returned the mocked API from `beforeEach` (Vitest treats returned functions as cleanup), producing extraneous diagnostics; the hook was corrected before the clean operations RED run.
2. `/tmp/final-fix-operations-red.log`: clean pre-fix TableOperations rerun, 5/5 failed. Pending/missing/failed kinds were incorrectly enabled, the direct view took a table operation path, and no independent metadata load existed.
3. `/tmp/final-fix-green.log`: after implementation, 99/100 tests passed across six focused files. The only remaining failure was the already-observed stale NOT NULL-label expectation; corrected to the current required-hint contract after coordinator confirmation.
4. `/tmp/final-fix-editor-red.log`: new SQL theme regression failed against original code because CodeMirror's dark-theme facet remained false after the app switched dark. Existing three SQL editor tests passed. The attached failed test left a view alive long enough for jsdom to report missing Range measurement APIs; the clean GREEN run has no such diagnostics.
5. `/tmp/final-fix-editor-green.log`: SQL editor 4/4 passed after the theme fix, without stderr diagnostics.
6. `/tmp/final-fix-focused.log`: final substantive focused command:
   `npm --prefix web test -- src/components/DataGrid.test.ts src/components/RowModal.test.ts src/components/TableOperations.test.ts src/components/SqlEditor.test.ts src/components/BulkEdit.test.ts src/views/TableView.test.ts src/views/SearchView.test.ts src/stores/catalog.test.ts`
   **8 files / 107 tests passed, exit 0**, no failures or stderr diagnostics.
7. `/tmp/final-fix-typecheck.log`: `npm --prefix web run type-check` — **passed, exit 0**, both Vue and Node projects. Node project includes `e2e/**/*.ts`, including the new final-regressions browser file.
8. `/tmp/final-fix-lint.log`: `npm --prefix web run lint` — **passed, exit 0**, full frontend ESLint with no diagnostics.
9. `git diff --check` — **passed, exit 0**.
10. A final added assertion checks that gutters and editor backgrounds retain similar luminance, explicitly catching a white gutter in dark mode. `/tmp/final-fix-editor-final.log` records the focused rerun after that test-only assertion: **4/4 SQL editor tests passed, exit 0**, no stderr diagnostics.

## Browser cases authored, not executed here

New file: `web/e2e/final-regressions.spec.ts`.

- Direct collapsed-sidebar view operations/drop.
- Selected duplicate keyless row cannot begin bulk update.
- MySQL/MariaDB signed and >24-hour TIME display, unrelated PATCH omission, edit and insert.
- Real expanded-sidebar competing catalog load preserves Search URL context and subsequent manual table choice.

All fixture mutations use the existing disposable-engine E2E harness, unique prefixes and finally cleanup. Existing `web/e2e/ux.spec.ts` was not edited by this worker. Browser outcomes, shared production build and whole-branch verification remain coordinator-owned and are not claimed here.

## Self-review and remaining limits

Checked encoded identifiers, PostgreSQL schema query propagation, unknown metadata action guards, stale response/mutation suppression, captured PK fields, existing bulk confirmation/partial-write contracts, exact temporal text/unchanged omission, manual Search selection retention, and SQL document/selection preservation. No additional blocking issue found in this worker's scope.

Deliberate limitation: the new bulk edit is unavailable on keyless tables, including those with only unique secondary indexes, because the existing available PK endpoint supplies the conservative reliable identity. This limitation is explained in the UI. Existing keyless single-row action semantics are unchanged and were explicitly excluded by the review.

## Coordinator browser follow-up: bodyless DELETE and PostgreSQL URL assertions

Coordinator's real MySQL run found the direct-view case failed in its diagnostic message: `expect(response.ok(), await response.text())` eagerly tried to retrieve the body of a successful HTTP 204 response, which Chromium does not expose through Network.getResponseBody. The assertion now uses the response status in its message and never reads the body. The post-drop URL expectation also explicitly preserves `?schema=public` for PostgreSQL, matching the existing fixture/router contract and coordinator's corresponding ux.spec.ts correction. No production code changed.

Coordinator reports the new MySQL keyless, duration and deterministic competing-search cases passed. These are coordinator-supplied browser results, not runs performed by this worker. This correction was inspected with `git diff --check`; browser/build execution and the already-running unit checks remain coordinator-owned.
