### Task 5: Bounded database-wide text search

**Files:** Create `web/src/views/SearchView.vue`, `SearchView.test.ts`, `web/src/lib/databaseSearch.ts`, `databaseSearch.test.ts`, `web/src/i18n/searchWorkflows.ts`; modify `web/src/router/index.ts` and register translation module.

**Interfaces:** Uses existing catalog/table definition/rows endpoints with encoded paths and catalog.schema. `searchDatabase` accepts selected tables and search text, onResult callback and AbortSignal; concurrency 2, max 20 selected tables per run, max 20 matches per text-column query, request timeout/stop bounded. Result includes table,column,rows and filter link using Task 3 query `filters` JSON. Literal `%`, `_`, and backslash escaped for LIKE. Reject empty query and clearly show per-column limits.

- [ ] Write failing tests: names containing spaces/%/slash keep correct URLs; literal search escapes metacharacters; selecting 21 tables is rejected; cancellation prevents subsequent work; one inaccessible table reports an error while others continue; result link preserves schema/filter.
- [ ] Run `npm --prefix web test -- src/lib/databaseSearch.test.ts src/views/SearchView.test.ts` and record RED.
- [ ] Implement bounded sequential-per-worker text searches with existing parameterized API and cancellation. Render selectable tables, visible scope/limit explanation, progress, errors, matching rows and table links. Restrict to supported text columns, never execute arbitrary generated SQL.
- [ ] Run focused tests/type checking and commit owned files.
