# Task 2 — database-aware transfer

Implemented in the shared `/home/hitslab/.codex/worktrees/ux-workflows/db-manager` checkout. No staging, commits, shared builds, browser/E2E runs or subagents.

## Delivered

- MySQL/MariaDB export honors the route database, including connections without a login database. Existing qualified catalog/data queries keep connection scope unchanged; integration checks verify `SELECT DATABASE()` remains the original login database. PostgreSQL still rejects database mismatch before output.
- Optional `ExportRequest.mode` supports omitted/`all`, `structure`, `data`; invalid modes fail before output. Structure excludes INSERTs; data excludes CREATE/DROP/view/FK definitions and views. Delimited formats remain data-only. UI presents mode and database/table context and accepts route `table` preselection.
- Optional multipart `columns` JSON array maps source positions to target names, with empty entries ignored. Server checks existence, uniqueness and a nonempty target selection. Omitted mapping preserves positional import and existing MySQL empty-field behavior. Mapped imports retain exact large integers, Unicode, empty strings, NULL (`\N`), escaped backslashes, binary hex bytes and omitted-column defaults; PostgreSQL schema context is preserved.
- Client CSV preview reads only the first 256 KiB, parses quoted delimiters/newlines/escaped quotes, renders at most 20 data records, and caps width at 200 source fields. Malformed records/settings produce visible errors. Presets, header selection, per-field target controls and optional legacy positional import are available.
- Stop uses AbortController and releases the response reader. Success requires terminal `ok:true`; premature EOF, parse errors and cancellation reject. Cancellation during arrival of a terminal chunk remains cancellation. Import results capture original database/table/type context and show confirmed completed rows/source bytes, explain partial work and refresh the catalog after possible writes. CSV progress reports applied batches, not buffered rows; `total=0` until completion.
- Root-requested connection identity addition: Go `DriverInfo.Port string` with `json:"port,omitempty"`; TypeScript `port?: string`. Both connect implementations normalize omitted ports to 3306/5432 and expose the connected port. Integration asserts serialization.
- `web/src/i18n/transferWorkflows.ts` exports `transferWorkflowMessages = { en: { transferWorkflows: {...} }, ru: { transferWorkflows: {...} } }`; common i18n integration belongs to the coordinator.

## Owned files

- `server/internal/api/handlers_transfer.go`, `handlers_transfer_test.go`.
- `server/internal/api/router_test.go`: only existing transfer test assumptions (PostgreSQL mismatch restriction) and CSV options deep comparison for the new slice.
- `server/internal/driver/types.go`: ExportRequest mode, CSVImportOpts columns/docs, and root-authorized DriverInfo port only.
- `server/internal/driver/csv_import.go`, `csv_import_test.go`: shared validated mapping resolver.
- `server/internal/driver/mysql/{dump.go,import.go,mysql.go,transfer_workflows_integration_test.go}`.
- `server/internal/driver/postgres/{dump.go,import.go,postgres.go,transfer_workflows_integration_test.go}`.
- `web/src/api/types.ts`: ExportRequest mode and root-authorized DriverInfo port only.
- `web/src/views/{ExportView.vue,ExportView.test.ts,ImportView.vue,ImportView.test.ts}`.
- `web/src/lib/{importFile.ts,importFile.test.ts,csvPreview.ts,csvPreview.test.ts}`.
- `web/src/i18n/transferWorkflows.ts`.

## RED / GREEN evidence

1. Focused Go API regressions initially failed: no-login/different-login MySQL export returned 400; invalid mode returned 200 and reached the driver. PostgreSQL restriction retained as a separate test.
2. Focused frontend regressions initially failed: EOF without success resolved, AbortError became a generic network error, selected MySQL export was disabled, route table was not preselected, and Stop did not exist.
3. CSV parser tests failed against a minimal empty parser for quoted comma/newline/Unicode, malformed quoting/width and truncation behavior. A later 201-field cap regression failed before adding the width guard.
4. Disposable integration initially failed for MySQL selected-database rejection and PostgreSQL structure mode emitting INSERT. Integration fixtures cover mapping order/ignored/unknown/duplicate targets and quoted Unicode. Expanded tests also reproduce mapped MySQL binary corruption: `0x00FF` was inserted as ASCII bytes (`0x307830304646`); typed hex binding fixes it. PostgreSQL/MySQL API hex strings use lowercase; test expectations were corrected after inspecting actual bytes.
5. An additional cancellation race regression failed because the terminal chunk resolved after abort; checking the signal after `reader.read()` fixes it.

## Verification

- `cd server && go test ./...`: PASS for every package, latest completed run after backend changes.
- `cd server && go test -tags integration ./internal/driver/mysql ./internal/driver/postgres -run TestTransferModesAndMapping -count=1`: PASS on disposable MySQL 8.4, MariaDB 11.4 and PostgreSQL 17. Covers mode contents/default, selected/no-login MySQL database, PostgreSQL mismatch/schema, exact mapped values/defaults/binary, invalid mapping, partial malformed input and cancellation after exactly 100 applied rows. Latest final rerun also includes actual views in mode fixtures.
- Focused frontend command on `src/lib/importFile.test.ts src/lib/csvPreview.test.ts src/views/ExportView.test.ts src/views/ImportView.test.ts`: 4 files / 26 tests PASS.
- `cd web && npm test`: 41 files / 256 tests PASS at the completed full-suite checkpoint (other owners subsequently added more tests). jsdom prints its existing download-navigation “Not implemented” message in export tests.
- ESLint on all owned frontend modules/views/tests: PASS.
- Latest full `npm run type-check`: no transfer errors after fixing mapping-select test wrapper typing. At that checkpoint only coordinator-owned `SearchView.test.ts:25` remained (`string | undefined` passed to URL). Earlier in-flight errors in TableOperations, navigationPreferences, TableView and ObjectTree were reported to root and were resolved by their owners.
- `git diff --check`: PASS at the completed checkpoint.

## Review and remaining limits

- Self-reviewed selected-database qualification, mapping validation/parameterization, generated/omitted targets, partial counts, abort/terminal races, stale metadata/route fetches and captured result scope.
- CSV preview is intentionally bounded; records after the preview window are validated during import. Files with no complete record in 256 KiB or more than 200 source fields cannot use this preview UI; the streaming API remains available.
- Imports remain non-atomic across statements/batches. Confirmed progress is a lower bound after cancellation/disconnection; an already running statement or a nontransactional engine may apply additional changes. The UI explicitly explains this and never claims rollback.
- Row-selection exports are data operations; combining `structure` mode with a row query is rejected. Existing dump paging consistency caveats remain.
- Coordinator still owns whole-branch type-check/lint/build, full DB integration/E2E and browser layout inspection.

## Review round 1 — I1 / I2 resolved

### Compatibility ruling and CSV format contract

Root explicitly approved an additive `ExportRequest.csvEncoding` option and central browser download ownership to retain old API defaults while fixing the browser export→mapped-import workflow.

| Operation | Omitted encoding | Explicit encoding |
| --- | --- | --- |
| MySQL/MariaDB CSV export API | Legacy raw strings, NULL as empty (unchanged) | `escaped`: NULL is `\N`, empty text stays empty, one extra backslash prefixes every literal leading backslash; binary remains `0x` hex. `legacy` explicitly retains old output. |
| PostgreSQL CSV export API | Existing escaped convention (unchanged) | `escaped` selects that convention; `legacy` emits raw values/NULL as empty. |
| Browser CSV/TSV downloads, including grid selections | Central `downloadExport` explicitly sends `csvEncoding:'escaped'` | Explicit caller `legacy` is preserved; SQL payloads are untouched. |
| Mapped CSV import | Existing mapped escaped interpretation (unchanged) | Multipart `csvEncoding:'escaped'` selects the reversible database format. `literal` preserves external CSV strings, including literal `\N`, leading backslashes and empty fields; it has no NULL marker. |
| MySQL import without `columns` mapping | Legacy positional behavior, including empty→NULL (unchanged) | Positional legacy behavior remains unchanged; use explicit column mapping for the escaped/literal options described above. |

The import screen exposes escaped database CSV versus literal external CSV and explains the marker/backslash/binary rules. The export screen explains the escaped format. Legacy CSV cannot recover a NULL/empty distinction that its original output omitted; retaining that old API format is intentional compatibility, while browser downloads now choose the reversible format explicitly.

### PostgreSQL data-only loading strategy

Data-only exports validate local foreign-key boundaries, include referenced parent tables transitively (matching structure mode's existing parent inclusion), and topologically order INSERT sections parent-first. Existing target constraints stay active. Cycles, including self-references, are explicitly rejected before any output; the error directs callers to Structure and data mode, whose existing restore installs constraints after loading rows. This is a conservative table-level strategy, not a promise to order rows within cyclic tables or provide atomic rollback.

### Regression evidence

- New disposable MySQL and MariaDB export→mapped-import fixtures initially failed for both whole-table and row-selection CSV: NULL became empty, literal `\N` became NULL, and doubled leading backslashes lost a character. The same fixtures now preserve NULL, empty text/binary, literal markers, one/two leading backslashes, binary bytes and quoted multiline Unicode.
- External literal mapped CSV initially decoded `\N` and removed a leading backslash; explicit literal encoding now preserves both. Equivalent escaped and literal tests pass on PostgreSQL.
- PostgreSQL structure→data restoration initially failed foreign-key checks for child-first requested order, child-only selection and whole-database selection. All now restore a real child pointing to parent `9007199254740993`. A cyclic schema initially emitted an apparently successful data dump; it now fails before bytes. Unit coverage additionally checks diamond dependencies, duplicate parent visits, self-references and missing dependencies.
- Browser download helper regressions initially omitted `csvEncoding:'escaped'` for comma/semicolon/tab exports; all now send it. Explicit legacy and SQL requests remain unchanged. Multipart literal encoding is serialized before the file.
- The minor delimiter finding is also fixed: preview accepts valid single-rune MySQL delimiters, including pipe and Unicode, while PostgreSQL enforces comma/semicolon/tab. A driver-capability change revalidates the preview. These tests were observed RED before implementation.
- Export component tests now stub anchor clicks, removing jsdom navigation noise.

### Verification after review fixes

- `cd server && go test ./...`: PASS.
- `cd server && go test -tags integration ./internal/driver/mysql ./internal/driver/postgres -run 'TestTransfer|TestExportCSV_SpecialValues|TestImportCSV_Basic' -count=1`: PASS. Disposable MySQL 8.4 + MariaDB 11.4 package run 63.541s; PostgreSQL 17 package run 14.021s. Includes existing legacy MySQL export/import regressions and all transfer integration fixtures.
- Focused frontend transfer suite: 5 files / 34 tests PASS, with no download-navigation errors.
- Full frontend type-check: PASS.
- Owned frontend ESLint: PASS.
- Full frontend test suite final result is recorded below after completion.

### Additional owned files in this round

`server/internal/dump/{writers.go,selection.go}` (shared reversible row codec and selection emitter), `server/internal/driver/postgres/{export_foreign_keys.go,export_data_test.go}` (parent order/cycle guard), `web/src/lib/{download.ts,download.test.ts}` (approved central browser export encoding). Original transfer files/tests/types and local `transferWorkflows` translations were updated as above. No common i18n file, build, E2E file, generated bundle, staging or commit was modified by this worker.

Full frontend run at the in-flight review checkpoint: 44 files, 42 passed / 2 failed; 286 tests passed / 8 failed. Transfer tests passed. Failures were in other owners' active changes and were reported to root; root subsequently reported their targeted suites green and requested no worker full-suite reruns:

- `ObjectsView.test.ts`: keeps partition SQL when refresh is cancelled; keeps the partition table and SQL when a table switch is cancelled; keeps partition SQL when route navigation is cancelled; requires discarding partition SQL before a destructive operation reloads it.
- `UsersView.test.ts`: creates an account and clears its password after saving; renames the selected account when its Save button is clicked; changes the selected account password when its Save button is clicked; saves account limits when its Save button is clicked.

`git diff --check`: PASS. Root owns the final stable whole-branch run. This worker's processes have finished and its transfer files are ready for review/commit.
