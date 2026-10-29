### Task 2: Database-aware export and usable CSV import

**Files:** Own `server/internal/api/handlers_transfer*`, `server/internal/driver/types.go` (ExportRequest/CSVImportOpts only), MySQL/Postgres dump/import implementations and transfer-specific tests; `web/src/views/ExportView*`, `ImportView*`, `web/src/lib/importFile*`, new CSV preview helpers/tests, `web/src/api/types.ts` (ExportRequest only), `web/src/i18n/transferWorkflows.ts`. Discover exact driver export/import files with rg before edits. Do not touch router.go or common locale files.

**Interfaces:** ExportRequest adds optional `mode:'all'|'structure'|'data'`. CSV mapping adds multipart `columns` JSON array of target column names in source-field order; empty target means ignored source column, duplicate/nonexistent names rejected. Mapping omitted preserves current positional behavior. `runImport(db,form,onProgress,schema?,signal?:AbortSignal)` requires terminal success. `table` route query preselects import/export.

- [ ] Add failing Go tests for export of other MySQL database, PostgreSQL mismatch, all/structure/data SQL content and invalid mode; CSV mapped/reordered/ignored columns, unknown/duplicate mapping and quoted Unicode rows. Add frontend tests for CSV quoted comma/newline parser, preselection, abort and premature EOF rejection.
- [ ] Run focused Go/frontend tests and record RED before implementation.
- [ ] Implement selected-database routing with driver-safe qualification/connection scope. Default mode is all; data skips DDL and views; structure skips data. Validate mapping server-side, preserve schema and exact values. Add bounded preview and mapping UI with CSV presets, cancellation and partial-work explanation.
- [ ] Run Go unit suite and frontend tests, plus targeted disposable DB integration tests for changed import/export behavior (coordinate build/E2E with root). Record GREEN.
- [ ] Self-review and report changed files and residual limits. Coordinator commits.
