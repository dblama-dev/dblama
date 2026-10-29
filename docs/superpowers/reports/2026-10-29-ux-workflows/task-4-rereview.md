### Spec Compliance

- ✅ Spec compliant within the requested fix-round scope. All four administrative submit controls now specify `attr-type="submit"` (`web/src/views/UsersView.vue:10`, `:16`, `:17`, `:18`).
- ✅ Partition drafts participate in dirty detection (`web/src/views/ObjectsView.vue:54`), table changes check before altering the active table (`:57`), reload establishes a clean baseline (`:66`), and destructive-operation confirmation checks discard before mutation (`:103`). Existing shared route/unload guards now receive the partition dirty state.
- ✅ The prior keyboard-selection test gap is resolved: `web/src/components/SqlEditor.test.ts:22` creates a real CodeMirror selection and asserts emission of only `SELECT 2;`, with the original document and model unchanged.

### Strengths

- `web/src/views/UsersView.test.ts:15` clicks the actual mounted Create button; new account-edit tests exercise actual Save-button activation and verify API methods, submitted values and password clearing.
- `web/src/views/ObjectsView.test.ts:87` onward covers cancelled refresh/table/navigation/destructive actions, accepted discard, post-save baseline and browser unload. The assertions verify source preservation and absence of unintended fetches/mutations.

### Issues

#### Critical (Must Fix)

- None.

#### Important (Should Fix)

- None; both prior Important findings are resolved. No new behavioral regression found in this fix diff.

#### Minor (Nice to Have)

- None in the reviewed implementation; prior real-selection coverage finding is resolved.

### Assessment

- **Task quality: Approved** for the fix-round scope.
- **Reasoning:** The production changes directly restore form semantics and prevent partition draft loss. The added tests exercise the browser-facing triggers that previously escaped coverage.
- **Checks:** read `review-33eab71..c61600e.diff` and the appended Task 4 report. Read existing validation logs: focused 3 files/23 tests passed; full frontend 44 files/297 tests passed; type-check and lint completed without diagnostics. Vitest emits only its runner performance advisory. No test reruns or source changes performed; only this review artifact was written.
- **Scope limit:** this approves the requested corrective diff; final build/browser E2E and cross-task structure-editor verification remain with the coordinator.
