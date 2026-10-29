### Task 1: Reliable row forms, related-record search and bulk editing

**Files:** Modify `web/src/components/RowModal.vue`, `RowModal.test.ts`; create `BulkEdit.vue`, `BulkEdit.test.ts`, focused `web/src/lib/row*.ts` helpers/tests and `web/src/i18n/rowWorkflows.ts`. Only these files owned; TableView integration belongs to Task 3.

**Interfaces:** RowModal retains existing props/events; full metadata must be ready before parent opens it. Optional `loading` prop may protect transitional state. BulkEdit props: `show`, `rowsPath`, `columns:ColumnDef[]`, `rowKeys:Record<string,unknown>[]`; emits `update:show`, `saved`. Existing PATCH row endpoint is used sequentially; parent refreshes after `saved` including partial writes. Local i18n module may be consumed directly using computed locale text helper until coordinator registers it.

- [ ] Write failing tests: edit `{name:'',notes:''}` with NOT NULL name saves only changed notes; insert defaults omit generated ID/status/timestamp; explicit empty string and NULL are preserved; untouched DECIMAL/BIGINT/binary remain exact; JSON invalid input gives actionable error; FK search requests a filter and includes selected/manual ID; bulk failure after one success reports one applied and stops.
- [ ] Run `npm --prefix web test -- src/components/RowModal.test.ts src/components/BulkEdit.test.ts` (missing new file may initially fail discovery; existing regression must fail behaviorally). Record RED.
- [ ] Implement `RowModal` value/NULL/DEFAULT controls and typed editors; reuse small pure payload helpers. Add remote debounced FK filtering with stale response guard and label/key options, referenced-table link and safe manual fallback. Implement confirmed BulkEdit; no new backend transaction claim.
- [ ] Run focused tests and frontend full unit suite; report unrelated concurrent failures by name, do not edit others' files.
- [ ] Self-review and write task report with changed files/TDD evidence. Coordinator commits after integration.
