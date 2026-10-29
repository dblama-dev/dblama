# Database workflows and usability

Status: implementation authorized by the user's “делай всё сам”, 2026-10-29.
Source: `docs/audits/2026-10-29/README.md`, against b9cc918.
Design details below are engineering decisions delegated to the agent, not individual user selections.

## Intent

Make existing database operations discoverable and reliable in everyday workflows, using phpMyAdmin's object context and direct actions as a reference. Deliver the audit's fixes, navigation improvements and proposed functionality without replacing the application's architecture.

## Constraints

- Preserve Go + Vue 3 + Naive UI + CodeMirror, English and Russian UI, MySQL/MariaDB/PostgreSQL capabilities and single-current-screen navigation.
- No new runtime dependencies, application database, password persistence, remote publishing or new database engines.
- Never run mutations against the user's existing databases; integration and browser tests use disposable containers only.
- Preserve exact DECIMAL/BIGINT values, binary transport, NULL/empty/default distinctions, schema scoping and encoded identifiers.
- PostgreSQL connections remain bound to the login database. MySQL/MariaDB export must honor the selected database under the same account permissions.
- Destructive actions require contextual UI confirmation. Cancellation stops future work and must not claim committed changes were rolled back.
- Persist drafts/bookmarks/preferences only locally, scope SQL to connection identity, user, database and schema; tolerate unavailable or corrupt browser storage.
- Public API additions remain backward compatible. Defaults preserve existing dump/import behavior.

## Approaches

1. Cosmetic fixes alone: inexpensive, but leaves hidden functions, DEFAULT errors and context loss. Rejected as insufficient for the approved audit.
2. Incremental workflow upgrades on existing components and APIs: chosen by agent. Allows focused regression tests and retains capability handling.
3. Replace layout/table libraries or clone all phpMyAdmin screens: adds migration cost and unrelated features. Rejected.

## Required outcomes

### Reliable row workflows

Fix all A1–A6 audit findings. Filter/sort/page state must survive tab switches and browser back via route query state. The rendered filters always describe the actual query. Single and bulk row deletion require confirmation with table/selection context. Long text uses bounded previews with full inspection; columns have useful widths and table-local scrolling. Pagination and total count are accessible above and below rows.

Row forms load full metadata before initialization, preserve typed changes, distinguish explicit value, NULL and DEFAULT, omit defaulted insert columns, allow empty NOT NULL text and hide/describe generated IDs. SQL expressions are executed only as server defaults, never interpolated from row input. Edit unchanged values does not submit them. Typed controls cover JSON validation/formatting, boolean, date/time and binary file input/preview without lossy numeric conversion.

Add `BulkEdit.vue`: explicit column selection and value/NULL controls, confirmed update of the captured selected row identities using existing PATCH requests. Sequential execution stops on first failure; reports applied count, retains error, and refreshes after any successful write. Do not claim atomicity. Parent API: props `show:boolean`, `rowsPath:string`, `columns:ColumnDef[]`, `rowKeys:Record<string,unknown>[]`; emits `update:show(boolean)`, `saved()`.

FK picker searches the server rather than only the first 200 local keys. Show a textual label with the key, preserve manual input and exact identifiers; provide a link to the referenced table. Compound FK fields remain manually editable with an explanatory hint if a safe composite picker is not implemented.

### Object context and navigation

Clickable server/database/schema/table breadcrumbs, clear table actions for Data, Structure, SQL query, Search, Insert, Import, Export, Operations and capability-gated Partitions. DDL display is labeled distinctly from query console. Import/export links carry `table` query parameter; console accepts `table` context, but never auto-executes SQL. Filter/search and insert actions may focus/open the existing grid and modal instead of duplicating screens.

Table Operations exposes existing single truncate/drop APIs with contextual confirmation. Database table list supports filtering, sorting, view badges, approximate counts, direct actions, select-all and existing bulk truncate/drop/copy/move operations. Preserve view constraints and refresh catalog after mutations.

Structure editor opens as a compact read overview with expandable per-column editor. Keep edits available and label separate save scopes (columns/options, indexes, FK); protect pending edits from navigation/reloads caused by saving another section. Sidebar is collapsible/resizable with locally saved preferences, recent/favorite tables and route-driven selection. Avoid hiding global navigation below a long tree. Shared editing/administrative surfaces use consistent controls and focus/error patterns.

### SQL workspace

Drafts persist on navigation and refresh in the correct scope. Ctrl/Cmd+Enter runs the selection when present, otherwise the whole SQL; explicit execute-selection behavior remains compatible. Add local named bookmarks with save/load/delete, and dialect-aware completion from current table/column metadata. Loading metadata must not overwrite an edited draft. Provide a shared CodeMirror editor for console and SQL object source where practical. Dirty object/structure editors warn before loss; local drafts do not require repetitive prompts.

### Data transfer

MySQL/MariaDB export accepts the route database even when login database is absent/different. PostgreSQL rejects a different database. Add `mode: 'all' | 'structure' | 'data'` to ExportRequest (omitted means all); structure omits INSERTs, data omits CREATE/DROP and schema objects, all preserves existing behavior. CSV export remains data-only. Include database/table context in UI and preselect a table from route.

CSV import gains bounded client preview, delimiter/enclosure presets and header choice, and column mapping to actual table columns. Support quoted delimiters/newlines and Unicode; report malformed preview instead of silently guessing. Server validates mapped target column names and duplicate mappings. Keep SQL and unmapped CSV imports backward compatible. Add AbortController-driven Stop; distinguish cancelled/partial from success. Require terminal `ok:true` before reporting successful import.

### Database-wide value search

Add a bounded search screen on database context for selected tables. Use existing metadata and parameterized row-filter API (one text column at a time is acceptable), limited concurrency, cancellation, per-table errors and limits. Never concatenate input into SQL. Search textual columns with literal contains semantics (escape LIKE metacharacters); allow supported engines to apply their own text collation. Return table/column, bounded matching row previews and links opening filtered table context. No unbounded full-server scan; show that results are limited.

## Verification and completion

Use TDD for behavioral changes. Run frontend unit tests/type checking/lint, Go unit and integration tests, production build and E2E matrix on all three engines. Add real browser workflows for DEFAULT, empty NOT NULL edits, filter restoration, draft restoration, table operations, and transfer mode/mapping. Manually inspect grid/structure/navigation at laptop and narrow widths. Document limitations honestly. Commit reviewable chunks to a local codex branch; no remote publishing. Final task review and whole-branch review must resolve important findings before completion.
