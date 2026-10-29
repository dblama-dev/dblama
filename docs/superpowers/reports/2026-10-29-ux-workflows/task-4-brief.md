### Task 4: SQL drafts, bookmarks, completion and common SQL editing

**Files:** Own `web/src/views/ConsoleView.vue`, `web/src/lib/useConsole*`, new SQL persistence/editor helpers and tests, `web/src/components/SqlEditor.vue`, `web/src/views/ObjectsView*`, `web/src/views/UsersView.vue`, `web/src/i18n/sqlWorkflows.ts`. Root registers all translation modules in `web/src/i18n/index.ts` once parallel modules exist.

**Interfaces:** SQL storage scope includes driver flavor/host/user/database/schema; connection host should include available port. Persistent local drafts load by scope, save on input, and handle storage exceptions. Console `table` query provides quoted starter SELECT only with no existing draft and never executes. Shared SqlEditor props `modelValue`, `dialect`, `schema?`; emits `update:modelValue`, `execute` with selected SQL or undefined. Object editors use the same editor with dirty navigation guard.

- [ ] Add failing tests for draft isolation/restoration/storage failure, bookmark save/replace/remove, keyboard selection execution and no overwrite after delayed completion metadata.
- [ ] Run focused tests and record RED.
- [ ] Implement bounded drafts/bookmarks and dialect-aware schema completion; integrate existing history, formatting, explain/cancel and Ctrl/Cmd+Enter. Add reusable SqlEditor to object sources and preserve accessible test locators where possible. Unify control styling in touched administrative screens.
- [ ] Run focused/full frontend tests and type checking; commit this task's owned files after other workers are ready or by exact paths.
