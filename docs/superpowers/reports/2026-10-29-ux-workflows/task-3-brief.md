### Task 3: Navigation, grid state, compact structure and existing operations

**Files:** Own `web/src/views/TableView*`, `DbView*`, `views/structure/StructureEditor*`, `components/DataGrid*`, `ObjectTree*`, `AppSidebar.vue`, `AdvancedNavigation.vue`, `layouts/BasicLayout.vue`, `components/TableOperations.vue` and tests, `web/src/lib/tableQuery*`, navigation/preference helpers/tests, `web/src/i18n/navigationWorkflows.ts`. Do not change RowModal/BulkEdit or ConsoleView/ImportView/ExportView or router/index.ts.

**Interfaces:** Export/import routes use query `table`; console query `table` identifies context. Query keys `filters` (JSON RowsFilter[]), `sort` (JSON RowsSort), `page`, `pageSize` retain state; preserve unrelated query keys incl schema/view. DataGrid takes controlled `filters` and emits existing apply-filters. Add `edit-rows` carrying captured row keys; parent renders Task 1 BulkEdit. TableOperations uses existing truncate/drop endpoints. New DB search link `/db/:db/search` implemented by Task 5.

- [ ] Add failing behavior tests for restored visible filter after tab switch, parsed route query/back, schema-preserving context links, default metadata loaded before opening insert, confirm-before-single-delete, select-all bulk table operations and capability-gated partitions. Test corrupt preference storage fallback and selection derived from route.
- [ ] Run focused component/helper tests and record RED.
- [ ] Implement route-backed query state; await full definition before opening row forms; add BulkEdit integration without changing its file. Make grid cells bounded/full-inspectable with useful widths/local scroll, sticky controls and pagination/count top/bottom. Provide accessible labels and Enter-to-filter.
- [ ] Implement context actions and clickable breadcrumbs, database list filtering/sorting/badges/quick actions, single/bulk truncate/drop with confirmation; make Sidebar collapse/resize and recent/favorites safe local preferences. Keep global links above long trees. Compact structure overview with expandable edits, guard dirty changes and independent save scopes without wiping other pending edits.
- [ ] Run focused and full frontend tests/type checking; report cross-task missing components while work is in progress. Document UI choices and any outstanding integration needs.
- [ ] Self-review and report; coordinator commits.
