### Spec Compliance

- ✅ Spec compliant for the scoped fix round: all three prior Important findings are resolved by `a454b27`, and the requested viewport-scroll and connection-port improvements are implemented.
- ✅ Full inspection: `web/src/components/DataGrid.vue:217` opens a captured row in the exact-value inspector; the row Preview action and modal column picker make even clipped values below 121 characters inspectable without adding duplicate cell text. `web/src/components/DataGrid.test.ts:450` exercises a short overflowing URL through the actual row action and picker.
- ✅ Clean metadata refresh: `web/src/views/structure/StructureEditor.vue:198` independently refreshes clean indexes and FKs from the fetched definition while preserving dirty scopes. Their baselines update only when their scope refreshes. `web/src/views/structure/StructureEditor.test.ts:559` covers an indexed-column rename and verifies clean unload behavior; the existing pending-scope preservation regression remains present.
- ✅ Accurate save baseline: `web/src/views/structure/IndexEditor.vue:119` captures a detached submitted model before POST and emits that exact model on success. `web/src/views/structure/StructureEditor.vue:138` records the submitted model as its baseline. The newer B edit therefore remains dirty after A finishes saving; both child and parent delayed-request regressions cover this behavior.
- ✅ Viewport scrolling: `web/src/components/DataGrid.vue:74` supplies a viewport-dependent body maximum height, bringing the table-local horizontal scrollbar within reach without traversing all 50 rows. `web/src/components/DataGrid.test.ts:465` checks the configured table bound.
- ✅ Connection scoping: `web/src/components/ObjectTree.vue:127` includes the separate port in the preference identity; `web/src/components/ObjectTree.test.ts:283` verifies favorite isolation and restoration when switching ports.
- ⚠️ Cannot verify from this fix diff: rendered laptop/narrow scrollbar positioning, final stable full-suite results, and the all-engine browser matrix. These remain controller-owned checks; they do not reopen the resolved code findings.

### Strengths

- `web/src/views/structure/IndexEditor.test.ts:10` checks both the actual POST payload and the saved-event payload, rather than merely checking that an event fired. The parent regression at `web/src/views/structure/StructureEditor.test.ts:580` additionally checks the real navigation guard after an edit during the delayed POST.
- `web/src/views/structure/StructureEditor.vue:198` keeps the metadata refresh decision independent for indexes and FKs, preserving the intended separate save scopes.
- `web/src/components/DataGrid.vue:217` shares the same exact-value inspection path between long-cell actions and the new row-level inspector, retaining plain cell text and a compact grid.

### Issues

#### Critical (Must Fix)

- None.

#### Important (Should Fix)

- None in the scoped fix diff. All three previously reported Important findings are resolved.

#### Minor (Nice to Have)

- No new actionable findings. The prior unrelated JSDOM navigation-notice cleanup remains outside this fix round.

### Assessment

**Task quality:** Approved.

**Reasoning:** The changes directly address each reported defect with behavior regressions, preserve the agreed component contracts, and add no runtime dependencies. No new correctness or maintainability issue was found in the fix diff.

**Checks:** Read the supplied `review-3098630..a454b27.diff` once and the appended fix-round evidence in `task-3-report.md` (66 focused tests, typecheck, lint, and whitespace checks reported passing). No tests rerun, no outside-diff code inspection, and no code/index/branch mutation; only this requested review artifact was written.
