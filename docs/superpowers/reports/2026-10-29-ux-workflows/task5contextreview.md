### Spec Compliance

- ✅ Scoped integration fix is compliant. `web/src/views/SearchView.vue:67-81` consumes string `query.table` only after catalog loading completes, selects only an exact available table name, and invalidates stale scope callbacks through watcher cleanup, catalog request identity and selection identity.
- ✅ `server/internal/sqlgen/sqlgen.go:125` corrects the previously identified malformed documentation without changing runtime behavior.

### Strengths

- `web/src/views/SearchView.vue:68-80` keeps context preselection within the scope watcher rather than reacting to every catalog refresh, so manual deselection is not reapplied automatically.
- `web/src/views/SearchView.test.ts:44-75` covers matching and missing context, schema/identifier encoding, manual deselection after a catalog refresh, and a stale response arriving after a newer scope and manual selection. Including the old table name in the new catalog makes the stale-scope regression meaningful.

### Issues

#### Critical (Must Fix)

- None found.

#### Important (Should Fix)

- None found.

#### Minor (Nice to Have)

- None introduced by this fix.

### Assessment

**Task quality:** Approved.

**Reasoning:** The change closes the table Search action integration gap with exact context matching and appropriate stale-response guards. The appended report contains focused 18-test, typecheck and lint evidence; no reported tests were rerun during review.

**Checks:** Reviewed only `task-5-context-fix.diff` and the appended section of `task-5-report.md`; no broader source inspection, git commands or test execution.
