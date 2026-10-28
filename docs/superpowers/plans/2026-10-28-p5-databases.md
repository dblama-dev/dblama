# P5 — databases, transfers and editor metadata

Status: implemented in shared `codex/p5-p6` worktree, 2026-10-28. Integration and commits owned by the coordinating agent.

## Delivered contracts

- New optional `driver.DatabaseManager`, `driver.TableTransfer`, `driver.MetadataProvider`; the base `Driver` remains compatible.
- `apiServer.registerDatabaseRoutes(chi.Router)` belongs inside authenticated `/api` routing. It registers `POST /databases` `{name,collation}`, `PATCH /databases/{db}` `{name}`, `DELETE /databases/{db}`, `POST /databases/{db}/table-transfer` `{action:"copy"|"move",tables,targetDB}`, `GET /metadata`.
- Mutations return 204. Invalid JSON, trailing documents, invalid names, empty/duplicate selections and identical source/target databases fail before driver calls. Table transfer requires both the optional interface and `Caps.MoveCopy`.
- Metadata shape: `{columnTypes:[{name,category,hasLength?}],collations:[{name,charset?,database}],operators:[{value,label,arity}]}`. `database` identifies collations usable during database creation. Operators match the browsing SQL generator whitelist.
- `DatabaseActions.vue`: optional `db` prop; absent means create, present means rename/drop. Emits `changed()`, `renamed(newName)`, `dropped(oldName)`. Drop requires typing the exact database name. Failed requests keep the form and display the server error.
- `TableTransfer.vue`: `db:string`, `tables:string[]`; emits `changed()`. Hidden unless `caps.moveCopy`. The target list excludes the current DB. Success offers native-router navigation to the target.
- `databaseMessages={ru:{databaseOps:...},en:{databaseOps:...}}` is merged by coordinator into global locale dictionaries.

## Safety and engine decisions

1. MySQL/MariaDB rename creates a new database with the source collation, copies definitions with `CREATE TABLE LIKE`, copies only writable columns under source/destination WRITE locks, drops all source tables in one statement while locked, then removes the empty source database. `DROP DATABASE` while locked was tested and rejected by both engines (1192), so this ordering is required.
2. All copied rows exist at the target before any source removal. Failed copying removes only destination tables created by this operation; an empty newly created target database may remain. After any destructive source step is attempted, destinations are retained even if cancellation leaves its commit status uncertain. Final cleanup failures explicitly identify the retained target/source.
3. MySQL database rename needs a DDL maintenance window: MySQL has no native atomic schema rename. The UI states this requirement. Concurrent DML on copied tables is blocked during copy/cutover; concurrent source-schema object creation is unsupported. A detected new table prevents the final source-schema drop.
4. Rename refuses views, routines, events, triggers, and foreign-key references before mutation. Table copying refuses views and tables with triggers or foreign keys rather than silently omitting objects that `LIKE` cannot preserve; the UI directs such transfers to SQL export/import. This is a deliberate safety boundary, not full object migration.
5. Native MySQL `RENAME TABLE` performs all requested table moves as one statement. Existing destination names are never overwritten. PostgreSQL cross-database move/copy is unavailable.
6. Copying preserves generated columns, explicit zero auto-increment IDs, and the next AUTO_INCREMENT counter (including gaps). A dedicated connection temporarily enables `NO_AUTO_VALUE_ON_ZERO`, restores its original SQL mode and unlocks before returning to the pool; failed session restoration discards that connection.
7. PostgreSQL DDL runs on a separate short-lived administrative pgx connection derived from the authenticated connection configuration, targeting `postgres` (or `template1` when necessary). Dropping/renaming the login database is rejected with explicit reconnect guidance. No active backend sessions are terminated.
8. PostgreSQL database locales are checked against server libc locales and SQL-literal escaped; identifiers use the dialect quoter. MariaDB generic UCA collation rows lacking a charset are excluded from the database-creation list.

## Additional P4 debts closed

- `dsn.go`: parse configs before pool creation; malformed parser errors cannot echo credential-bearing DSNs. Coordinator changed `postgres.go` to call `openDatabase(cfg)`.
- Coordinator made the database-size query conditional on CONNECT privilege. A real restricted role can list a database whose CONNECT grant was revoked without triggering `pg_database_size` privilege failure.
- Negative sequence increments preserved during creation (`0` keeps the existing default of 1).
- PostgreSQL column comments read back through `col_description`, preserving comments through structure editing/export consumers.
- `execDDL` always defers rollback, including panic paths.
- CREATE rejects duplicate named indexes and multiple PRIMARY definitions before execution. FK rendering normalizes/whitelists referential actions and validates mapping counts/names. CREATE additionally calls the shared `driver.ValidateForeignKeys(...,false)` supplied by the object workstream.

## Verification evidence

TDD RED observed before implementation for missing optional contracts/methods, missing API registration, missing frontend modules, unsafe duplicate indexes, missing panic rollback, negative sequence increment, and zero auto-increment ID corruption.

- `GOCACHE=/tmp/db-manager-audit-go-cache go test -tags integration ./internal/driver/mysql -run '^TestDatabaseOperations$' -count=1`: PASS on `mysql:8.4` and `mariadb:11.4` after zero-ID/counter regression fix (12.287s).
- `... go test -tags integration ./internal/driver/postgres -run 'TestDatabaseOperations|TestP4DatabaseDebts' -count=1`: PASS on `postgres:17` (2.602s).
- `... go test ./internal/api -run '^TestDatabaseOperations' -count=1`: PASS.
- `... go test ./internal/driver/postgres -run 'TestConnectMalformed|TestCreateRejects|TestExecDDLRolls' -count=2`: PASS.
- `npm test -- src/lib/databaseOps.test.ts src/components/DatabaseActions.test.ts src/components/TableTransfer.test.ts`: 7 tests PASS.
- Whole frontend suite: 28 files / 172 tests PASS; jsdom emitted two existing “navigation to another Document” notices.
- `npm run type-check` and `npm run lint`: PASS.
- Full Go suite and golangci were attempted during concurrent work. Initial full-suite failures were unrelated in-progress `NewStatementStream`/stream syntax and FK-route RED tests; coordinating agent must rerun after all workstreams land. Database and PostgreSQL package tests passed. Own database lint findings (exported comments, panic-test conversions) were corrected; final combined lint remains a coordinator gate.

## Portable SQL export follow-up

Coordinator expanded this workstream to views, selections, generated values and portable FKs.

- `IncludeViews=false` retains base-table-only SQL defaults. With views enabled, empty selection exports all local tables/views; selected views include their transitive local table/view dependencies. Selected tables additionally include their local FK-parent closure. Dependencies outside the exported database/schema fail before output; unsupported routine/sequence/table-function view dependencies fail explicitly.
- Shared `dump/view.go` lexes SQL identifiers separately from strings, dollar strings and comments. It removes source qualifiers only at relation references / MySQL three-part column references and preserves alias-qualified expressions and source-looking literal text. Expression `FROM` inside EXTRACT/TRIM is distinguished from a SELECT's FROM clause.
- Views emit DDL only, in dependency order. In-place dumps drop dependent views before base tables. `IF NOT EXISTS` skips existing views using conditional engine-native DDL rather than replacing their definitions.
- PostgreSQL export-specific CREATE statements omit inline FKs. All tables, row data and sequence state are emitted first, followed by ALTER TABLE ADD FOREIGN KEY with local unqualified references. The SQL tab's ShowCreateTable path is unchanged. A single scoped DROP TABLE list supports cycles; no CASCADE can destroy unrelated target objects.
- MySQL/MariaDB whole SQL dumps save/disable/restore session FOREIGN_KEY_CHECKS and SQL_MODE. Generated columns are omitted from SQL INSERT projections while CSV keeps them. Explicit zero AUTO_INCREMENT IDs survive both whole-table and SQL selection exports. Import connections are physically discarded by the coordinating workstream after arbitrary session statements.
- MySQL read values now use RFC3339Nano. SQL literals and keyset timestamp bounds retain fractional seconds; DATETIME(6) round-trips with `.123456` intact.
- View security semantics follow-up: PostgreSQL security_invoker/security_barrier flags and CHECK OPTION are reconstructed from pg_class.reloptions; MySQL SQL SECURITY INVOKER is preserved from information_schema.VIEWS. Restricted-reader fixtures verify restored invoker views cannot gain access to underlying tables. Default DEFINER ownership belongs to the importing user; original definers/ACL grants are not copied.

Additional checks:

- `go test ./...`: PASS after the concurrent workstreams stabilized.
- `golangci-lint run`: 0 issues after export/FK changes.
- `go test -tags integration ./internal/driver/mysql ./internal/driver/postgres -run '^TestExportPortableForeignKeys$' -count=1`: PASS (MySQL/MariaDB 13.291s; PostgreSQL 1.840s). Fixture covers alphabetically earlier child, cyclic/composite keys, two in-place restores, selected-parent closure, external FK refusal before bytes, resolved target-schema catalog references, source preservation and PostgreSQL refusal to cascade into unrelated target dependencies.
- `go test -tags integration ./internal/driver/mysql ./internal/driver/postgres -run 'TestExportViewsGeneratedAndSelection|TestExportPortableForeignKeys|^TestDatabaseOperations$|TestP4DatabaseDebts' -count=1`: PASS before the final view-security extension (46.464s / 8.359s).
- Existing affected export regressions `TestExportSQL_RoundTrip|TestExport_Options|TestReview_DumpAndPool`: PASS (23.084s / 4.087s).

Remaining scope boundaries: table rename/copy safety restrictions above remain intentional; cross-database PostgreSQL table transfer remains unsupported. Portable dumps do not copy roles/grants, original view definers, routines/triggers/events, PostgreSQL user-defined types, or FK attributes absent from the v0.1 ForeignKey contract (such as deferrability). Concurrently modified source data still has the documented pagination/snapshot limitation. The coordinator owns final whole-repository integration, build, UI verification and commits.

Final workstream checkpoint after view-security preservation:

- `GOCACHE=/tmp/db-manager-audit-go-cache go test ./...`: PASS, every package.
- `GOCACHE=/tmp/db-manager-audit-go-cache golangci-lint run`: **0 issues**.
- `GOCACHE=/tmp/db-manager-audit-go-cache go test -tags integration ./internal/driver/mysql ./internal/driver/postgres -run 'TestExportViewsGeneratedAndSelection|TestExportPortableForeignKeys' -count=1`: PASS — `mysql:8.4` + `mariadb:11.4` 35.776s; `postgres:17` 8.706s.
- Restricted-reader tests first reproduced source INVOKER denial versus incorrectly permitted restored DEFINER access. After preservation, source and restored views both deny unauthorized underlying-table reads. PostgreSQL restored security_invoker/security_barrier flags are additionally checked through pg_class.

All owned source files are stable and available for the coordinator's unified commit. No independent commits were created.
