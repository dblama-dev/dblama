# E2E workflows implementation plan

Spec: docs/superpowers/specs/2026-10-29-e2e-workflows-design.md
Authorization: user explicitly requests autonomous scenario design and E2E implementation.
Workspace: existing clean .worktrees/p4-postgresql, branch codex/table-editor-e2e.

## Global constraints
Real browser/Go/database for E2E; no API mocks. Disposable databases only.
Preserve existing user work and running services. Diagnose before fixes, reproduce
with a failing test, then change minimal production code. Keep tests independent.

## Task 1: Harness and session/structure tests
Install locked Playwright; runner owns disposable DB and app lifetime; retain
failure trace/screenshots. Test actual clicks for autoincrement and new indexes,
then repair checkbox and draft index editing. Assert persisted definition and
server-generated IDs. Add component regression using DOM clicks.

## Task 2: Data and SQL scenarios
Implement independent row, filtering/pagination, SQL recovery and import/export
workflows. Root diagnoses real failures and fixes them with evidence.

## Task 3: Database/object/monitor scenarios
Implement database create/rename/drop cancellation, view lifecycle and monitor
read paths. Root diagnoses failures and applies minimal fixes.

## Task 4: Integration and verification
Run each E2E against all three engines, full Vitest, lint/typecheck, Go unit tests
and build. Add CI matrix and documented local commands (Docker and external DB).
Request independent branch review; resolve significant findings. Commit verified
changes, report actual coverage and remaining gaps.

## Review focus
Cleanup on failure, unique resources, test isolation, assertions on persisted
values not only notifications, no fake network, waits tied to responses, no
sleep/retries hiding faults, CREATE indexes in same DDL, compatibility of existing
index save, no changes to live app or user DB.

## Observed regressions and fixes

- Real click could not clear AUTO_INCREMENT: replaced radio with checkbox, retained
  exclusivity. Unit and browser regression observed failing before fix.
- New table had no index editor: added draft indices in the CREATE payload; invalid
  index drafts block Save. Tests assert same create request and persisted PK/IDs.
- PostgreSQL serial disable kept nextval default: clearing generation now clears
  that default. Unit and PostgreSQL browser regression observed RED → GREEN.
- Sidebar initialization overwrote route database catalog: route wins login DB,
  and sidebar selection navigates consistently. Route/race unit and admin E2E.
- Go null result slices crashed result renderer: normalized nullable rows/columns,
  tested DDL, DML and empty SELECT plus mixed statements in browser.
- Header sort event read wrong field: use Naive DataTableSortState.columnKey;
  component test clicks real headers, E2E checks actual order and exported rows.
- MariaDB default UCA collation alias absent from metadata catalog blocked rename:
  safely quoted COLLATE is validated by the server. Native database evidence,
  failing integration regression, both-engine integration and browser checks.
- Old type-check command inspected an empty project: now explicitly checks app
  and Node/E2E projects; repaired actual type-only diagnostics, removed deprecated
  baseUrl (relative paths are already explicit).

## Independent review

No blocking findings. Documented sequential runs per checkout. Remaining minor
limitations: a SIGTERM to only the shell wrapper can defer cleanup until its child
exits; test cleanup navigation can change failure screenshot contents (trace keeps
all actions). Full privileged administration/large data/multi-browser testing and
PostgreSQL identity transitions beyond serial disable are outside this suite.
External database provisioning belongs to the operator; no user database was used.

## Final verification — 2026-10-29

- Complete Chromium suite, no retries: MySQL 8.4 **13/13**, MariaDB 11.4 **13/13**,
  PostgreSQL 17 **13/13**. All scenarios ran together for each engine.
- Vitest: **199/199** tests in 33 files. Existing jsdom navigation warnings remain;
  no failed tests. ESLint and explicit app + Node/E2E TypeScript checks pass.
- `go test ./...` passes. Database operations/default-collation rename integration
  regressions pass against both mysql:8.4 and mariadb:11.4.
- `make build`, `actionlint .github/workflows/ci.yml`, `bash -n scripts/check-e2e.sh`,
  and `git diff --check` pass. Existing bundle-size warning remains.
- No E2E-labelled containers remain after the full matrix. Normal failures were
  also observed to clean up the runner-owned container during RED runs.
- Independent review found no blocking issues. Remaining limitations listed above.
- Native external-DB mode is provided but was not separately exercised; the matrix
  used disposable database containers and a native Go application process.
