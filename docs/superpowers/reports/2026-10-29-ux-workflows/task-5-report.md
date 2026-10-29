# Task 5: Bounded database-wide text search

## Delivered

- Added `/db/:db/search` under `BasicLayout`, matching Task 3 navigation.
- Added `SearchView.vue` with database/schema context, selectable table scope (at most 20), literal text input, visible resource/collation/type limits, progress, Stop, table/column errors, bounded row previews, and links to matching table rows.
- Added `databaseSearch.ts`: sequential requests per worker, two workers maximum, 20 rows per text-column request, encoded database/table/schema identifiers, only known text columns, no generated arbitrary SQL, per-request AbortController with 15-second default timeout (30-second enforced maximum), cancellation between requests, no late result callbacks after abort, and continuation after inaccessible tables/columns.
- Kept `api/client.ts` unchanged. Local cancellable GET preserves `X-Requested-With: XMLHttpRequest`, same-origin credentials and request signal, and bounds response/body consumption with its timeout.
- Result links preserve schema and serialize `RowsFilter[]` to Task 3's JSON `filters` route query; they select data view, first page, page size 20.
- Exported `searchWorkflowMessages` with English/Russian `searchWorkflows` namespaces. Coordinator owns registration in `i18n/index.ts` and was notified.

## Approved implementation adjustment

The original brief proposed client-escaped LIKE values. Inspection found that `sqlgen` emits parameterized LIKE with no explicit ESCAPE clause. MySQL's `NO_BACKSLASH_ESCAPES` mode makes implicit backslash escaping unreliable. Coordinator explicitly authorized a backward-compatible `CONTAINS` operator:

- Frontend sends raw literal text as the row filter value.
- SQL generator renders `column LIKE <bound placeholder> ESCAPE '!'`.
- Bound pattern escapes `!` to `!!`, `%` to `!%`, and `_` to `!_`, with contains wildcards added only by the server.
- Backslash remains literal because `!` is explicitly the LIKE escape character.
- Existing LIKE and other operators retain their behavior.
- Driver operator metadata already derives from `sqlgen.AllowedOperators()`, so no driver metadata changes were needed.
- Navigation owner added CONTAINS to the frontend row filter operator whitelist.

## Additional integration fix requested by coordinator

Moved `catalog.schema` synchronization from `router.beforeEach` into successful `router.afterEach`. A cancelled dirty-editor navigation previously changed schema before its leave guard could reject it. Authentication remains in beforeEach; schema capability setup remains available there. Regression test rejects a schema-changing route in beforeResolve, proves the old schema remains, then confirms successful search navigation changes it.

## Owned files

- `web/src/views/SearchView.vue`
- `web/src/views/SearchView.test.ts`
- `web/src/lib/databaseSearch.ts`
- `web/src/lib/databaseSearch.test.ts`
- `web/src/i18n/searchWorkflows.ts`
- `web/src/router/index.ts`
- `web/src/router/router.test.ts`
- `server/internal/sqlgen/sqlgen.go`
- `server/internal/sqlgen/sqlgen_test.go`
- `server/internal/sqlgen/contains_test.go`
- This report.

No files were staged or committed. No subagents, builds, E2E, user-database queries, or database mutations were run.

## TDD and verification

1. Search tests first failed because new modules did not exist. Added API/template placeholders and reran to establish behavioral RED: 9 tests failed for missing behavior (one expected placeholder rejection also appeared as an unhandled rejection during fake-timer advancement).
2. `go test ./internal/sqlgen` established RED for CONTAINS: both MySQL/PostgreSQL tests rejected the unsupported operator, and AllowedOperators expected the new entry.
3. Added literal operator implementation. SQLgen tests passed, checking exact MySQL/PostgreSQL SQL, bound metacharacter/backslash/malicious text, and empty-value rejection.
4. Search implementation GREEN: `npm --prefix web test -- src/lib/databaseSearch.test.ts src/views/SearchView.test.ts` — 2 files, 9 tests passed.
5. Router regression RED: failed because cancelled schema navigation changed catalog.schema from `first` to `blocked`. It also exposed the missing search route.
6. Focused GREEN: `npm --prefix web test -- src/lib/databaseSearch.test.ts src/views/SearchView.test.ts src/router/router.test.ts` — 3 files, 15 tests passed.
7. `npm --prefix web run type-check` — passed after fixing the test's possibly undefined href typing.
8. `npx eslint src/lib/databaseSearch.ts src/lib/databaseSearch.test.ts src/views/SearchView.vue src/views/SearchView.test.ts src/i18n/searchWorkflows.ts src/router/index.ts src/router/router.test.ts` from web — passed.
9. Full frontend suite run once: `npm --prefix web test` — 43 files, 273 tests, 271 passed and 2 failed. Both failures were reported immediately to coordinator and are in navigation ownership:
   - `src/lib/tableQuery.test.ts > buildRowsQuery > exposes the backend operator whitelist in a stable order`: expected whitelist omitted newly added CONTAINS.
   - `src/components/TableOperations.test.ts > never truncates a view and drops it using the view endpoint`: expected DELETE `/api/databases/app/views/report`; component used DELETE `/api/databases/app/tables/report`.

## Limits and remaining whole-branch verification

- Large table scans and exact-count queries can be expensive despite bounded result count; UI explicitly explains this and allows Stop.
- Search covers known textual scalar types, omitting arrays, binary, numeric, JSON and custom types.
- Row previews show the matched column, capped at 240 characters; linked table view provides complete rows.
- No real-engine execution or browser visual check was run by this worker, per coordinator instruction. SQL semantics are covered by dialect SQL/argument tests; whole-branch engine integration/E2E remains coordinator-owned.
- Full-suite failures above need the owning agent's fixes and coordinator verification before declaring the entire branch green.

## Task 6 integration follow-up: table-context preselection

Coordinator identified that Task 3's table Search action passes `?table=<name>`, while the search screen did not consume that context. Added exact-match preselection only after the requested database/schema catalog load succeeds. Unknown/non-string table contexts remain unselected. The watcher now includes table query context and invalidates pending preselection on scope changes/unmount. It checks catalog request identity and selection identity before applying context; it does not reapply context when the user deselects a table or the shared catalog refreshes.

Added regressions for encoded identifiers and schema-preserving catalog URL, available exact context, missing context, manual deselection surviving catalog reload, and an older scope response arriving after a newer catalog and manual selection. The stale test deliberately includes the old table name in the new catalog, so mere name availability cannot hide a stale preselection bug.

TDD evidence: the new SearchView tests first produced RED with 2 failures (available context remained unchecked, and newer context remained unchecked). After implementation, focused helper/view/router verification passed 18 tests across 3 files. The strengthened stale-response test was then rerun with all 5 SearchView tests passing. `npm --prefix web run type-check` and scoped ESLint for SearchView/component test passed. No full suite, build, E2E or commits were run during this follow-up.

Also corrected the malformed QuoteIdent empty-identifier documentation to say “returns two backticks”; runtime SQL behavior is unchanged.
