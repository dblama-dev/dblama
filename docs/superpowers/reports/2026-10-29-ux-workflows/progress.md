# SDD ledger — plan: docs/superpowers/plans/2026-10-29-ux-workflows.md

Base: b9cc918. Worktree: /home/hitslab/.codex/worktrees/ux-workflows/db-manager. Branch: codex/ux-workflows.
User explicitly delegated all decisions, artifacts and execution on 2026-10-29. No repeated design/plan approvals required.

## Preflight

| Tasks | Interface/file check | Ruling |
|---|---|---|
| 1 / 3 | RowModal metadata initialization, BulkEdit props/events | 3 awaits full TableDef; 1 retains contracts and owns form; 3 integrates form |
| 2 / 3 | Import/export context links | Query table string preselects; both retain schema |
| 3 / 4 | SQL action | Console query table; no automatic execution |
| 3 / 5 | Filter result links | filters=JSON RowsFilter[], page/pageSize/sort preserve schema |
| 1 / 2 / 3 / 4 / 5 | Translations | Separate modules; root registers all after files exist |
| 1 / 2 | Shared API types | 2 only ExportRequest additions, 1 consumes existing ColumnDef unchanged |
| 1 | Tests vs form contracts | coherent; sequential bulk reports partial count, not atomic |
| 2 | Tests vs transfer contracts | coherent; omitted options preserve compatibility |
| 3 | Tests vs navigation contracts | coherent; route state source of truth |
| 4 | Tests vs persistence contracts | coherent; root owns editor/SQL |
| 5 | Tests vs search limits | coherent; existing parameterized filters |
| 6 | Shared build outputs | root alone runs E2E/build, engines sequential |

Ruling: Native managed worktree authorized by autonomy; preserve original audit in original checkout and copy into branch.
Ruling: Three independent owners may implement in parallel per dispatching-parallel-agents; no shared-file edits, coordinator alone stages/commits.
Ruling: Frontend existing PATCH endpoint supports bulk edit via sequential requests, with explicit partial results; no new fake transaction API.
Ruling: Broad audit includes typed editing and database search although brief final answer omitted them; deliver full report scope.

Baseline: make backend-test web-test passed; 199 frontend tests, all Go unit packages pass. Existing jsdom navigation warnings (2) noted, not introduced here. Log /tmp/dbm-ux-baseline.log.

## Tasks
- [x] 1 Row workflows
- [x] 2 Transfer
- [x] 3 Navigation/grid/structure
- [x] 4 SQL workspace
- [x] 5 Database search
- [x] 6 Integration/review/verification

Initial checkpoint: design check was running; implementation had not started.

Design check: /root/design_check reported no important unanswered questions or contradictions. Spec/plan committed b548b9a.
Tasks 1/2/3 implementing in parallel (row_workflows, transfer_workflows, navigation_workflows); Task4 root implementation.
Ruling: DriverInfo exposes optional port for persisted SQL scope separation; Task2 owns this additive shared type change.

Implementation checkpoints: Task2 2292fe0, Task5 8494a8d, Task3 9e86ffc, Task4 aab9303. Task1 final FK name search underway. Task2/5 review dispatched; Task3/4 packages ready. Root running whole E2E; no build agents.
Ruling: CONTAINS backend operator with explicit ESCAPE '!' replaces planned client LIKE escaping, because NO_BACKSLASH_ESCAPES makes client approach unreliable. Raw search input remains parameterized; route operator whitelist includes CONTAINS.
Ruling: schema state changes only after successful navigation, to preserve dirty-editor cancellation.
Task4 fresh focused validation 31 tests/5 files pass. Browser E2E initial run shows closed details remove cell accessible names; nav owner notified, correction pending. Task2 reviewer identified PG data-only FK order and CSV NULL/backslash roundtrip issues, fix round pending report.
Task 5: complete (commits 2292fe0..8494a8d, review clean). Integrator verified translations registered aab9303 and CONTAINS route parser in9e86ffc. Minor: comment formatting, catalog request not cancelled (store generation guards existing).
Task 2: fix round1/5 dispatched to original transfer_workflows with two important findings (CSV sentinel roundtrip and PG data-only FK order). Ruling: additive csvEncoding option preserves legacy API default; UI requests escaped consistently via central download helper.
Task1 implementation commit5f36f63; review_rows dispatched with task package. Task3 browser fix in progress (cell accessible names + bounded long preview). Initial E2E13:7pass6fail; five share grid defect, one changed filter accessible label adjusted. New ux.spec.ts adds defaults/filter persistence, SQLdraft/bookmarks/hotkey, literal search/resultlink checks.
Task3 grid browser fix committed4b6d5d4; final focused25pass. Actual manual latestbuild confirms compact rows and named cells, external fullvalue modal works with keyboard. Manual still finds horizontal scrollbar only below50rows; bound tableheight pending Task3 review/fix.
Task6 MySQL E2E latest16/16 pass (/tmp/dbm-e2e-mysql2.log). Additional defaultonly+bulk E2E added after reviewer found APIemptyinsert mismatch; pending run. Shared manual disposable database dbm-ux-manual localhost32803/ux_audit, app18090 session74419. Do not leave resources after final.
Task1 review twoImportant: emptyinsert backend and PGdate display. Fixround1 dispatched originalowner; Task4 review twoImportant: NButton submitattrs and partitiondraft dirtyguard. Fixround1 dispatched fresh sql_fixes (root was implementer); realCodeMirror selection test included. Task3review waiting slot.
Task 1: fix round1/5 (2 addressed,0 open; commits5f36f63..04ae293), re-review clean. Task1 complete.
Task 2: fix round1/5 (2 addressed,0 open; commits04ae293..33eab71), re-review clean. Task2 complete. Additional errcheck-only API test cleanup verified GoAPI+golangci0issues, awaiting commit.
Task 4: fix round1/5 (2 Important+selection test addressed,0 open;33eab71..c61600e), re-review clean. Task4 complete.
Task3 fixround1 underway originalowner: inspection of clippedshortvalues, cleanmetadata refresh after columnsave, inflightindexdirtybaseline. Include portscope and viewportboundscroll minor improvements.
Root stable Go unit+297 frontend tests pass. golangci initial7errcheck tests fixedbyowner; finalmake lint to rerun. Full backend integration session85985 underway. UX browser expanded5/5 pass (/tmp/dbm-e2e-ux.log), additional longvalue/account form tests nowadded.
Task 3: fix round1/5 (3Important+2minor addressed,0open;3098630..a454b27), re-review clean. Task3 complete.
Task5 contextual tablequery follow-up3098630 approved by scopedreview;18focusedtests pass. Task5complete.
Root manual1366x768 final table finds4action buttons wrap at240px, made presentation-only width300/nowrap adjustment. Defaultgenerated fields no longer say required; NOTNULL shown only explicitvalues. Final wholebranch review will include these tiny polish changes. No test contract changed.
Stable checks after task3/contextfix:306tests/45files pass; fullGo unit, golangci0issues, eslint,typecheck pass (/tmp/dbm-ux-final-checks2.log). MySQL20/20browserpass, MariaDB20/20browserpass. PG matrixnowrunning.
Wholebranch review final_review (gpt6astra high) dispatchedb9cc918..d111a55; pendingfindings include searchsharedcatalograce, keylessbulk scope, MySQLTIME durationnativecontrol. Do not fixwave until completefindings.
Ruling: integrationtarget timeout30m replaces default10m because fullfresh-container MySQL/MariaDBpackage exceeded10m with no priorfailures. First runPG+API passed; mysqltimeout atTestExportCSV_SpecialValues. New fullrun /tmp/dbm-ux-backend-integration-final.log session36228. Cost: CI can wait longer for a realhang; individual container checks unchanged.
Finalreview4Important recorded in final-review.md; ONE fixagent final_fixes owns completewave inclfocusedunit and newfinal-regressions.spec.ts (no E2E/build). BASE97058a9. Four: keylessbulk unselectedduplicates; MySQL/MariaDBTIME durations; viewkindmetadata withcollapsedsidebar; Search querytable same-scope catalogcompetition.
PG E2E firstuxcase failed because test assumed firstphysicalrow remainsfirst afterUPDATE; updated row moves physically inPG. Root corrected locator to exactPK1, not productbehavior/assertion weakening. Existingux.spec root-owned; finalfixagent newbrowserfile separate.
PG first20browserresult17pass2fail1capabilityskip. Both failures testassumptions: updatedrowordering and finalDBURLincludespreservedschema=public. Root fixed selectors/expectedscopedURL; no appdefect in these. Remaining20newtests finalwave still to run.
Manual snapshots saved docs/superpowers/reports/2026-10-29-ux-workflows/{data-after,data-wide,structure-after,structure-narrow,sql-workspace}.jpg. At1366x768, collapsedwide and600px tested. SQLkeyboardexecution/bookmarksuccess verified UI. Darktheme uncovered inheritedCodeMirror lighttokencontrast; saved sql-dark-before-fix.jpg and added to same ongoing finalfixwave (SqlEditortheme +text/selectionpreservationtest). No newfixwave dispatched.
Ruling: bulk edit requires PK to prevent unselectedduplicatewrites; existing single keyless actions retain documented matchall semantics. Cost: keylesstables require SQL or single-row workflow instead of newbulkform.

Final fixes committed4aec995 (one wave). All four final Important findings plus manualdarktheme corrected. Focused107 tests, full317tests/45files, Go unit, golangci0issues, ESLint,typecheck all pass (/tmp/dbm-ux-checks-wave.log). Full backendintegration exit0: API118.409s, MySQL/MariaDB1198.085s, PG111.521s. MySQL browser23passed/1testdiagnosticfailure on empty204body; corrected eagerbodyread and PGschema expectation in samewave, regressionfile rerun underway. Single scopedfinal rereview dispatched97058a9..4aec995. Darktheme actualbrowser visualconfirmed and screenshot saved sql-dark-after.jpg. Manualapp stopped and owned dbm-ux-manual container removed; usercontainers untouched.

Single scoped final rereview Approved: all four Important fixed, darktheme and204/PGtest corrections approved, no material residual. MySQL full23pass plus corrected4case rerun4pass; combined24cases verified with unchanged production. MariaDB full24running. No secondwave needed.

MariaDB final browser24/24pass, exit0, 2.4m. PostgreSQL final24scenario matrix started.

Final verification complete: PostgreSQL22passed/2capabilityskips (TIME duration +MySQLaccountform), exit0. MySQL24cases verified via23passfull +4passfocused after204diagnosticcorrection; MariaDB24/24pass. Full317frontendtests/45files, allGounit, integrationallpackages, golangci0issues, ESLint/typecheck, productionbuild anddiffcheck passed. Usercontainers untouched, all owneddisposableDB/app resources stopped. Finalscope checklist andstable artifacts copied; retain localcodex/ux-workflows per approvedplan, no push/publish. TemporarySDD planworkspace will be removed only after stable artifactcommit.
