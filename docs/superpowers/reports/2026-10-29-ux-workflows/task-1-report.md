# Task 1 report — reliable row forms and bulk editing

Status: implemented and verified in the shared ux-workflows worktree, including Important review fixes. No files staged or committed by this worker. No build, browser E2E, or mutations against user databases. Review integration fixtures used fresh disposable containers only.

## Owned files

- `web/src/components/RowModal.vue`
- `web/src/components/RowModal.test.ts`
- `web/src/components/BulkEdit.vue` (new)
- `web/src/components/BulkEdit.test.ts` (new)
- `web/src/lib/rowValues.ts` (new; exercised through real component behavior tests)
- `web/src/i18n/rowWorkflows.ts` (new)
- `server/internal/driver/mysql/data.go` (review fix)
- `server/internal/driver/postgres/data.go` (review fix)
- `server/internal/api/row_defaults_integration_test.go` (new; review regression on actual handler + drivers)
- This report.

No TableView/DataGrid/shared locale index files were edited by this worker.

## Implementation

RowModal now tracks explicit value, NULL, and DEFAULT separately. Insert defaults and untouched generated/auto-increment identities are omitted, allowing server defaults to execute. Explicit empty text and NULL override defaults correctly. Edit payloads contain only changed fields, including when an existing NOT NULL string is empty. Generated columns are described and never serialized; auto-increment IDs expose a server-default choice with optional explicit values.

Pure row helpers own initialization, binary transport, changed-field payloads, JSON validation, and representable-value validation. BIGINT/int8/DECIMAL/numeric remain text, so exact identifiers and decimal values do not pass through Number. Binary hex uses the existing `$binaryHex` wrapper. Existing binary values are compared after transport normalization, preventing unchanged byte previews from generating PATCH changes.

Editors cover enum, boolean, date, time, local datetime/timestamp, long text, JSON, and binary hex/file input. JSON validation reports the affected column with corrective guidance and an optional formatter. Binary files are read as bytes and encoded without text reinterpretation. Date/time editors do not normalize original data into payloads unless the user changes them. Timestamp values containing time-zone information retain a text editor to preserve offsets.

Foreign-key selectors make debounced server requests with a literal CONTAINS filter on a textual label by default, with an explicit selector for exact-ID search. They preserve selected/manual identifiers alongside remote results, show a textual label plus key, and provide encoded links to the referenced table. Request generation/per-field counters prevent old responses from replacing current search results. PostgreSQL referenced schemas and MySQL referenced databases are respected. Request failures retain the manual-ID path with an explanatory hint. Compound foreign keys remain manually editable and explicitly describe that limitation.

BulkEdit implements the agreed `show`, `rowsPath`, `columns`, `rowKeys` props and `update:show` / `saved` events. It captures identities/path when opened, requires explicit column selection, supports value/NULL updates, validates before confirmation, and confirms the table plus selected count with a non-atomicity explanation. PATCH calls execute sequentially and stop at the first error. Successful writes are counted; errors and progress remain visible. Stop prevents future requests after the current request completes. Any successful write emits `saved`, including partial/cancelled outcomes, for parent refresh. The component deliberately does not close itself on partial outcomes.

## Integration notes

- Import `rowWorkflowMessages` from `web/src/i18n/rowWorkflows.ts`; it exports `{en:{rowWorkflows:{...}},ru:{rowWorkflows:{...}}}`. Root registers it in the shared i18n index. Components also consume the module through the active locale so they work before registration.
- TableView must await complete metadata before opening RowModal; this is owned by navigation. Existing optional `loading` prop also guards transitional initialization.
- BulkEdit's parent should refresh on `saved` without automatically closing its modal, so partial-write errors/progress remain visible.
- FK search defaults to a textual name/title/label column when available, with exact-ID mode and manual keys still available. Tables without a suitable text label use exact-key search. Composite keys remain manual.

## TDD evidence

Skills read/applied: test-driven-development, its writing-good-tests reference, systematic-debugging, and verification-before-completion.

Initial diagnosis traced failures to three original code paths: all empty non-nullable values disabled Save, every insert field was serialized despite defaults, and FK choices were a one-time first-200-key fetch. Behavioral regressions were added before changing those paths.

1. Initial RED command: `npm --prefix web test -- src/components/RowModal.test.ts src/components/BulkEdit.test.ts`.
   - Log: `/tmp/task-1-red.log`.
   - Result: 2 failed files; 5 failed behavioral RowModal tests and 25 passed tests, plus missing BulkEdit import.
   - Confirmed real failures: unchanged empty NOT NULL name blocked notes edit; generated/default fields were sent; explicit-default controls were absent; malformed JSON was posted; selected FK identifier/text label were absent.
   - Existing exact decimal/binary behavior was retained and additional exact BIGINT/DECIMAL/binary regression coverage added.
2. First implementation preserved old tests except three deliberately superseded contracts: empty NOT NULL text was formerly disallowed and generated identities formerly submitted `id:null`. Those expectations were updated to the approved semantics.
3. Typed editor/link RED: `/tmp/task-1-typed-red.log` showed three concrete failures: false boolean lacked a selected option, timestamp remained a generic text control, and the FK navigation link used an API-shaped path. Fixes addressed those exact causes.
4. Bulk validation RED: `/tmp/task-1-bulk-validation-red.log` showed empty NOT NULL numeric updates reached confirmation. Shared value validation now blocks that with the column named. A stale-FK regression initially raced component mount; awaiting initial mount settling fixed the test setup, and no production workaround was added.
5. Final focused GREEN: `/tmp/task-1-green.log` — 2 files, 38 tests passed. This includes partial writes stopping after one applied row, captured identity isolation, cancellation before future requests, exact binary file bytes, stale FK response suppression, and encoded PostgreSQL-schema navigation.

## Final verification

- `npm --prefix web test`: **40 files / 246 tests passed**, exit 0; `/tmp/task-1-full-suite.log`.
- `npm --prefix web run type-check`: **passed**, exit 0.
- Owned-file ESLint: **passed**, exit 0.
- `git diff --check` for owned files: **passed**.

An intermediate typecheck caught an owned select-option interface/index-signature mismatch, fixed by using a type alias. It also reported concurrent TableOperations.vue emit-overload, navigationPreferences.test.ts unused import, and TableView.test.ts nullable-access errors; the final typecheck passed after the owning agent addressed those files. No unrelated files were edited to fix them.

The full test run emitted three jsdom diagnostics, `Not implemented: navigation to another Document`, without failing tests. Shared build/E2E and disposable-container database roundtrips remain coordinator verification work.

## Self-review

Checked original binary snapshots versus transport payloads, untouched exact numbers, default omission versus explicit empty/NULL, sequential partial writes and cancellation, generation guards, schema/database scoping, encoded table/database identifiers, generated-column handling, and parent event semantics. No blocking issue found in owned scope. UI confirmation explicitly says prior writes remain after later failure or cancellation; no transaction/rollback claim is made.


## Follow-up: remote FK name search

Coordinator requested completion of label-name search after the backend added parameterized literal `CONTAINS`. Read `server/internal/sqlgen/sqlgen.go` and `contains_test.go`: callers send raw search text; the server binds a LIKE pattern and escapes `%`, `_`, and `!` using an explicit escape character.

Added a behavioral regression with 200 initial parent rows that omit the sought name. The mock returns the matching record only when the component sends `name:CONTAINS:Wild%_!name`. The original component failed because it sent `id:=:Wild%_!name`; RED is recorded in `/tmp/task-1-fk-label-red.log`.

Implementation now remembers a usable textual label column from the parent page, prioritizing name/title/label, excludes array columns, and defaults subsequent searches to its `CONTAINS` filter. An explicit Name/ID mode control retains exact-key lookup. Manual IDs and an existing selected BIGINT key remain available across results; choosing the remote name result saves its exact key. Queued searches await initial metadata and retain request-generation guards. The stale-response regression now starts two searches after metadata is loaded and resolves the older search last, ensuring the name-search changes preserve response ordering protection.

Follow-up changed only `RowModal.vue`, `RowModal.test.ts`, `rowWorkflows.ts`, and this report. No shared index, backend, build, E2E, commit, or stage operations.

Verification after follow-up:

- Focused RowModal/BulkEdit run: **39 tests passed**; `/tmp/task-1-fk-label-green.log`.
- Full frontend run after final changes: **43 files / 274 tests passed**, exit 0; `/tmp/task-1-fk-label-full-suite.log`.
- Full frontend type-check: **passed**, exit 0.
- Follow-up owned-file ESLint and `git diff --check`: **passed**.
- Same three jsdom navigation diagnostics appeared without failures.


## Review fix round 1 — default-only endpoint compatibility and PostgreSQL DATE

Read `task-1-review.md` and traced both Important findings across component/driver boundaries before editing.

### Actual default-only inserts

Both drivers rejected empty maps before SQL generation. MySQL/MariaDB now execute a qualified, quoted `INSERT INTO ... () VALUES ()`; PostgreSQL permits an empty map to reach its existing qualified `INSERT INTO ... DEFAULT VALUES` path. Existing generated-identity RETURNING/LastInsertID handling remains active, and missing required values still receive normal database constraint errors. Empty database/table checks remain unchanged.

Added `server/internal/api/row_defaults_integration_test.go`, `TestAPI_DefaultOnlyRowInsert`, using real router login, HTTP POST/GET handlers and actual driver connections on fresh mysql:8.4, mariadb:11.4 and postgres:17 testcontainers. The test first posts `{}` into a table containing a generated identity, literal default, CURRENT_TIMESTAMP, nullable column, defaulted base value, and generated expression; it verifies persisted results and returned ID. It then verifies explicit empty text/NULL overrides and identity-only/no-identity default inserts, including LastInsertID zero for a table without generated identity. Containers are terminated by test cleanup.

RED: `go test -tags integration ./internal/api -run '^TestAPI_DefaultOnlyRowInsert$' -count=1 -v` failed on **all three engines**, actual endpoint returned HTTP500 instead of200; `/tmp/task-1-default-insert-red.log`.

GREEN after driver fixes: same command passed on **all three engines**, exit0; `/tmp/task-1-default-insert-green.log` (27.35s test time).

### Native DATE display

The new regression uses the actual PostgreSQL RFC3339 DATE representation, `2026-10-29T00:00:00Z`, and first checks the native input displays `2026-10-29`. It then edits a different text field and checks DATE is absent from PATCH. RED: `/tmp/task-1-review-date-red.log` showed the native control's value was empty.

RowModal now extracts the original calendar-date prefix only for the native DATE display. It does not parse through JavaScript Date, convert time zones, or alter fields/original edit snapshots. Datetime/time behavior remains unchanged.

GREEN: focused RowModal/BulkEdit **40 tests passed**; `/tmp/task-1-review-date-green.log`.

### Final review-round verification

- Real HTTP integration on fresh MySQL/MariaDB/PostgreSQL: **passed**.
- Full `go test ./...`: **passed**.
- Full frontend `npm --prefix web test`: **44 files / 286 tests passed**, exit0; `/tmp/task-1-review-full-suite.log`.
- Full frontend type-check: **passed**.
- RowModal/RowModal-test ESLint: **passed**.
- Owned-file `git diff --check`: **passed**.
- Latest frontend suite emitted **no navigation diagnostics**; the earlier three jsdom diagnostics were absent after concurrent test-owner changes.

No shared build, E2E, commit or stage operation was run by this worker. Both Important review findings are addressed with behavioral RED/GREEN evidence and actual database-backed handler/driver coverage.
