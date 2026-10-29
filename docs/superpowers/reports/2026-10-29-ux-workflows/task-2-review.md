### Spec Compliance

- ❌ Issues found: exact-value transfer is incomplete for the default MySQL/MariaDB CSV workflow (`server/internal/driver/mysql/import.go:209`, `web/src/views/ImportView.vue:157`); PostgreSQL data-only dumps are not safely restorable with ordinary foreign keys (`server/internal/driver/postgres/dump.go:130`, `server/internal/driver/postgres/dump.go:241`).
- ✅ The scoped MySQL route is passed into Export, PostgreSQL mismatch remains rejected, and invalid modes are rejected before streaming (`server/internal/api/handlers_transfer.go:55`, `server/internal/api/handlers_transfer.go:74`, `server/internal/api/handlers_transfer.go:99`). Mapping validation rejects unknown/duplicate/nonselected targets while preserving omitted mappings (`server/internal/driver/csv_import.go:8`). Cancellation and missing terminal success reject (`web/src/lib/importFile.ts:108`).
- ⚠️ Cannot verify from this task diff: coordinator integration of `transferWorkflowMessages`, final whole-branch type-check/build/lint, and browser workflows/layout. The report explicitly leaves these with the coordinator (`.superpowers/sdd/2026-10-29-ux-workflows/task-2-report.md:43`, `:52`).

### Strengths

- Applied-batch progress is substantially more trustworthy than reporting buffered CSV records; both drivers check context between records and emit final totals only after successful flush (`server/internal/driver/mysql/import.go:172`, `server/internal/driver/postgres/import.go:151`).
- The response reader is cancelled/released on terminal success and failures, with an explicit abort-after-read race regression (`web/src/lib/importFile.ts:111`, `web/src/lib/importFile.test.ts:18`).
- Bounded preview preserves strings and parses quoted multiline Unicode without numeric conversion; metadata/mapping requests use generation guards (`web/src/lib/csvPreview.ts:4`, `web/src/views/ImportView.vue:212`).

### Issues

#### Critical (Must Fix)

- None identified.

#### Important (Should Fix)

- **I1 — MySQL CSV export no longer round-trips through the default import.** `server/internal/driver/mysql/import.go:209` interprets `\N` as NULL, keeps empty fields as empty strings, and removes one leading backslash from doubled backslashes; `web/src/views/ImportView.vue:157` enables this mapping mode by default. However, unchanged MySQL `exportDelimited` passes raw values to the shared CSV writer (`server/internal/driver/mysql/dump.go:402`), whose `CSVValue` exports NULL as an empty field and passes literal strings through (`server/internal/dump/writers.go:106`). Exporting a nullable text value and importing the application's CSV now silently changes NULL to empty; an exported literal `\N` becomes NULL; an exported doubled leading backslash loses one character. Preserve backward-compatible positional behavior while establishing an explicit, consistent encoding between MySQL CSV export and mapped import. Add a real export→mapped-import round-trip regression containing NULL, empty, literal `\N`, and leading backslashes on MySQL and MariaDB. Existing integration tests manufacture the mapped encoding by hand, so they miss this mismatch.

- **I2 — PostgreSQL data mode emits child rows before parent rows with active constraints.** `server/internal/driver/postgres/dump.go:130` bypasses table preparation in data mode, and `:241` emits ordinary table-by-table INSERTs in the retained table order. A schema with `a_child(parent_id REFERENCES z_parent(id))`, exported as structure then data and restored into an empty target, inserts `a_child` first and fails immediately. Structure mode has already installed the FK, and unchanged `ImportSQL` executes the statements directly without disabling/defering constraints (`server/internal/driver/postgres/import.go:62`). This breaks the new structure/data workflow for a common valid schema; even changing requested table order can trigger it. Implement a valid loading strategy with dependency-aware ordering and an explicit safe treatment/rejection of cycles, without pretending rollback or silently discarding constraints. Add a disposable PostgreSQL structure→data restore test whose child sorts before its parent; content-token tests do not establish restorability.

#### Minor (Nice to Have)

- `web/src/lib/csvPreview.ts:5`: the preview accepts only comma/semicolon/tab despite displaying an editable separator control and MySQL's existing single-rune separator support. A previously usable pipe-delimited MySQL CSV is blocked before submission. Align frontend validation with the selected driver's accepted separators or expose only supported controls.
- `.superpowers/sdd/2026-10-29-ux-workflows/task-2-report.md:41`: the reported frontend test output still contains jsdom download-navigation errors. Stub download navigation in export tests so genuine runtime errors remain visible; this is reported existing noise, not evidence of a newly introduced production failure.

### Assessment

**Task quality:** Needs fixes.

**Reasoning:** The implementation handles routing, mapping validation, bounded parsing, and partial/cancelled progress carefully. The two untested end-to-end transfer combinations above can either corrupt values or produce a dump that cannot be restored, so they block approval.

### Checks

- Read brief, full report, authoritative design, and full review package; the first combined tool response was truncated, so the package was reread in bounded chunks. No reported tests were rerun and no git commands were run.
- Concrete outside-diff risk checks: CSV codec compatibility → shared `dump/writers.go` and delimited export bodies; FK restore semantics → both `export_foreign_keys.go` helpers and PostgreSQL `ImportSQL`; stale catalog scope → `web/src/stores/catalog.ts`. The dump and import hunks were cut off mid-function, so only the necessary missing function portions were inspected separately.
