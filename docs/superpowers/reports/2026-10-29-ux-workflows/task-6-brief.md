### Task 6: Integration, browser regressions, review and delivery

**Files:** `web/e2e/ux.spec.ts`, shared fixtures only as needed, README/CHANGELOG and this plan, `docs/superpowers/reports/2026-10-29-ux-workflows.md`; no changes to existing test expectations merely to hide regressions.

- [ ] Integrate all translation modules and interfaces; add E2E regressions for A1–A6, default row insertion, bulk edit, table operations, SQL draft/bookmark and transfer modes/mapping. Existing behavior must continue to work on all three engines.
- [ ] Run `make backend-test web-test lint`, `make backend-integration`, `make build`; fix actual failures, then run `scripts/check-e2e.sh mysql`, `mariadb`, `postgres` sequentially.
- [ ] Conduct browser inspection using disposable database only: grid long TEXT at 1366×768 and narrow window, compact structure, table navigation, modal types and SQL editor. Save before/after screenshots to report directory, stop temporary resources.
- [ ] Obtain fresh task reviews and whole-branch review per superpowers; fix Important/Critical issues and rerun affected validation. Record minor residual limitations explicitly.
- [ ] Update implementation report, check every spec outcome against changes/tests, commit changes and retain local reviewable branch. Do not push/publish. Return concrete results, verified test counts and artifact paths.
