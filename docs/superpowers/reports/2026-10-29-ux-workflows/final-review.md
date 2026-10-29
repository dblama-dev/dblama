# Whole-branch UX review

Reviewed base `b9cc918` through `d111a55`, the approved design/plan, Task 1–6 reports, previous reviews/fix rulings, and current integration sources. Also inspected the coordinator's uncommitted Makefile/README change extending the integration package timeout to 30 minutes. Review is read-only apart from this requested report; no tests, build, browser actions, database writes, or subagents were run.

## Strengths

- The integrated row workflow awaits full definitions before opening forms, omits server defaults, compares normalized unchanged values, preserves exact-number/binary transport, and refreshes after successful partial bulk writes.
- Transfer changes preserve omitted-option API behavior, validate mappings server-side, require terminal import success, and qualify the selected MySQL database. PostgreSQL data-mode dependency ordering and the documented cycle limitation address the prior restoration finding.
- Route-backed filters, schema-preserving links, dirty-editor guards, scoped local SQL persistence, parameterized literal CONTAINS searches, and stale-response guards are present across the integration boundaries reviewed.
- E2E additions exercise real endpoints for defaults, bulk editing, CSV mapping, export modes, table operations, draft restoration, literal search, long-value inspection, and actual account-form buttons. Existing scoped fixes are supported by focused regressions rather than only assertion changes.

## Issues

### Critical

None identified.

### Important

1. **[P1] Bulk editing keyless tables can update rows outside the captured selection.** `web/src/components/DataGrid.vue:336–339` forwards the selected rows' `__pk` values without requiring a primary key. For keyless tables, those values are the complete original row (`DataGrid.vue:179–181`), not a unique identity. `BulkEdit.vue:75–78` then sends a normal PATCH and counts each successful request as one applied row. Both existing drivers explicitly update *every* row matching that predicate (`server/internal/driver/mysql/data.go:284–290`, `server/internal/driver/postgres/data.go:412–419`). Reproduction by code contract: a keyless table contains two identical rows; select only one and change a column through Bulk Edit. Both rows change, while the modal reports “1 of 1 rows applied.” This violates the new feature's selected-row scope and honest applied-count requirement. Gate the new bulk operation on a reliable unique identity (the available PK metadata is the conservative option), or implement an equivalent safe identification strategy. Add a duplicate-keyless-row regression ensuring the unselected row is never silently changed. This finding concerns the newly added bulk action, not a request to redesign all pre-existing keyless row actions.

2. **[P2] Native TIME controls reject valid MySQL/MariaDB duration values.** `web/src/components/RowModal.vue:89` unconditionally chooses `<input type="time">` for TIME, and `:13` binds the database value directly. MySQL/MariaDB TIME can represent durations such as `36:00:00` or `-01:30:00`; native HTML time inputs accept a time of day, sanitize these existing values to an empty display, and prevent entering these valid duration values. The old generic text path supported them. Preserve a text/duration editing path for these engines/types, including inserts, rather than selecting a clock-only control based solely on the SQL type spelling. Cover an existing negative/>24-hour value, unchanged PATCH omission, and editing/inserting such a value. The original field state remaining intact means an unrelated edit does not itself erase the TIME; the regression is hidden display and inability to edit valid values.

3. **[P2] View operations depend on a sidebar that the new collapsed layout removes.** `web/src/components/TableOperations.vue:27` determines object kind solely from `catalog.tables`; `:29–34` treats missing metadata as a base table. `web/src/layouts/BasicLayout.vue:8` now unmounts AppSidebar when collapsed, and `:25` automatically collapses it below 720px. On a fresh direct `/db/<db>/table/<view>?view=operations` load with collapsed sidebar, no ObjectTree catalog request runs, and TableView's own initialization fetches only PK/rows. The catalog remains empty. Truncate is incorrectly enabled, and confirmed Drop omits `isView=true`. The server handler therefore invokes DropTables instead of DropViews (`server/internal/api/handlers_tables.go:106–117`), so normal view deletion fails. Resolve authoritative object kind in the table/operations workflow independently of sidebar visibility, retain database/schema scope, and keep destructive controls disabled while kind is unknown. Add a direct-load/collapsed-sidebar view regression, not only a test with a prefilled catalog.

4. **[P2] Search table context can disappear when sidebar catalog loading supersedes the same-scope request.** `web/src/views/SearchView.vue:73–79` applies `?table=` only if its own catalog request remains the latest request. On a fresh expanded-sidebar load, `ObjectTree.vue:162–175` independently fetches databases and then the same table catalog. If this second catalog request starts before SearchView's first request settles, `catalog.requestID` changes and SearchView permanently skips preselection. Completion of the newer successful same-scope catalog request triggers no retry; the visible search screen has zero selected tables despite its table-scoped URL. The current isolated SearchView tests never mount the competing loader. Distinguish a superseding request for the same scope from a navigation to a different scope: initialize context once from the successful current-scope catalog, while retaining the existing guards against manual selection changes and stale scopes. Add an integrated or controlled competing-catalog-request test for this ordering.

### Minor

No additional minor findings proposed. Previously accepted documented limits remain recorded in the task reports.

## Declined to judge

- Background continuation of a confirmed bulk update after navigating away: considered, but set aside because navigation-as-cancellation is not specified and the operation already snapshots its target; the executor should decide whether navigation should stop future requests or preserve a visible background-operation status.
- Editing column definitions during an in-flight save of those same columns: considered, but set aside as pre-existing behavior outside the newly required protection from saving another section. The new index submitted-snapshot fix and independent section preservation were reviewed.
- Existing duplicate/keyless semantics of individual Edit/Delete/Copy actions: outside this new bulk-edit finding; no claim that this branch introduced those existing endpoint semantics.
- Final screenshots, resource cleanup, final report/checklist completion, all-engine E2E completion, and the 30-minute backend integration rerun: coordinator work explicitly still in progress, not silently waived or treated as passing.

## Verification evidence and Task 6 assessment

Read `/tmp/dbm-ux-final-checks2.log`: Go unit packages pass; 45 frontend files / 306 tests pass; golangci reports 0 issues; ESLint and both TypeScript checks finish without diagnostics. Read the initial backend integration log and coordinator update: API/PostgreSQL integration pass; the MySQL package exhausted the prior default 10-minute package timeout while creating fresh containers. The coordinator's timeout increase does not suppress a test assertion or reduce coverage; the rerun outcome remains required. Coordinator reports MySQL browser matrix 20/20 passing; remaining engine/browser/manual evidence is still pending.

Plan coverage is broad and the previously scoped Important findings are resolved, but the four integration/type/selection defects above are not covered by the existing passing checks. Complete fixes and targeted regressions, then finish the coordinator-owned final verification and artifacts.

## Assessment

**Ready to merge? With fixes.**

The architecture and most required workflows are integrated coherently, with substantial behavioral and real-engine coverage. The new bulk action can exceed the user's selected rows, and three ordinary context/type paths remain broken; these Important findings should be resolved before the branch is declared complete.
