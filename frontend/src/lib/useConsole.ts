import { computed, ref, watch } from 'vue'
import { loadDraft, saveDraft } from './sqlWorkspace'

import { withSchema } from './schema'
import { api } from '../api/client'
import type { QueryResult } from '../api/types'
import { loadHistory, pushHistory } from './consoleHistory'

/** db getter and localStorage history key of the console's server. */
export interface ConsoleScope {
  db: () => string
  serverKey: string
  schema?: () => string | null
}

/** All state and actions of the SQL console except the CodeMirror
 * wiring: execution (all / selection / EXPLAIN), cancel, results and the
 * per-server history cycle behind Alt+Up/Down. Extracted from
 * ConsoleView so the orchestration is testable without the editor. */
export function useConsole(scope: ConsoleScope) {
  const draftKey = computed(() => JSON.stringify([scope.serverKey,scope.db(),scope.schema?.() ?? '']))
  const sql = ref(loadDraft(draftKey.value) ?? '')
  const draftSaved = ref(true)
  let switchingScope = false
  let requestVersion = 0
  watch(draftKey, key => {
    switchingScope = true
    sql.value = loadDraft(key) ?? ''
    switchingScope = false
    results.value = null
    error.value = ''
    executing.value = false
    queryID.value = ''
    requestVersion++
  }, {flush:'sync'})
  watch(sql, value => {
    if (!switchingScope) draftSaved.value = saveDraft(draftKey.value,value)
  }, {flush:'sync'})
  const executing = ref(false)
  const results = ref<QueryResult[] | null>(null)
  const error = ref('')
  /** queryID of the run currently executing (or the last one). */
  const queryID = ref('')
  const history = ref<string[]>(loadHistory(scope.serverKey))
  /** Position inside history while cycling; -1 means "not cycling". */
  let historyPos = -1
  /** Editor text before the first Up of a cycling session; restored
   * when Down walks past the newest entry. */
  let historyDraft: string | null = null

  /** Forgets an in-progress cycle: used when the doc is replaced from
   * outside the Alt+Up/Down keys (e.g. a history dropdown pick) and when
   * a new run starts, so a stale position cannot skip entries. */
  function resetHistoryCycle(): void {
    historyPos = -1
    historyDraft = null
  }

  /** Runs one SQL batch: queryID per run, backend defaults for maxRows
   * (1000) and timeoutSec (60). On success the executed sql is recorded
   * in the server history; on failure the message is stored and the old
   * results are dropped (the backend sends no partial results). */
  async function run(text: string): Promise<boolean> {
    const trimmed = text.trim()
    if (trimmed === '' || executing.value) return false
    executing.value = true
    const request = ++requestVersion
    error.value = ''
    results.value = null
    queryID.value = crypto.randomUUID()
    resetHistoryCycle()
    try {
      const payload = await api<{ results: QueryResult[] }>(
        'POST',
        withSchema(`/api/databases/${encodeURIComponent(scope.db())}/query`, scope.schema?.() ?? null, true),
        { sql: trimmed, queryID: queryID.value },
      )
      if (request !== requestVersion) return false
      results.value = payload.results ?? []
      pushHistory(scope.serverKey, trimmed)
      history.value = loadHistory(scope.serverKey)
      return true
    } catch (e) {
      if (request === requestVersion) error.value = e instanceof Error ? e.message : String(e)
      return false
    } finally {
      if (request === requestVersion) executing.value = false
    }
  }

  /** Execute all (selection undefined/empty) or only the selection. */
  function execute(selection?: string): Promise<boolean> {
    const hasSelection = selection !== undefined && selection.trim() !== ''
    return run(hasSelection ? selection : sql.value)
  }

  /** EXPLAIN of the selection or the whole text via the dedicated
   * /api/query endpoint (the server wraps the sql itself). */
  async function explain(selection?: string): Promise<boolean> {
    const hasSelection = selection !== undefined && selection.trim() !== ''
    const trimmed = (hasSelection ? selection : sql.value).trim()
    if (trimmed === '' || executing.value) return false
    executing.value = true
    const request = ++requestVersion
    error.value = ''
    results.value = null
    resetHistoryCycle()
    try {
      const payload = await api<{ result: QueryResult | null }>('POST', withSchema('/api/query', scope.schema?.() ?? null, true), {
        db: scope.db(),
        sql: trimmed,
      })
      if (request !== requestVersion) return false
      results.value = payload.result === null ? [] : [payload.result]
      return true
    } catch (e) {
      if (request === requestVersion) error.value = e instanceof Error ? e.message : String(e)
      return false
    } finally {
      if (request === requestVersion) executing.value = false
    }
  }

  /** Cancels the current run; the pending execute() rejects with the
   * driver's cancel message, which lands in `error`. */
  async function cancel(): Promise<void> {
    if (queryID.value === '') return
    await api('POST', `/api/query/${queryID.value}/cancel`)
  }

  /** Alt+Up: the next older history entry (starting at the most recent),
   * or null when the history is exhausted. The first Up of a session
   * snapshots the editor text so Down can restore it later. */
  function historyPrev(): string | null {
    if (history.value.length === 0) return null
    if (historyPos === -1) historyDraft = sql.value
    historyPos = Math.min(historyPos + 1, history.value.length - 1)
    return history.value[historyPos] ?? null
  }

  /** Alt+Down: the next newer entry; once past the newest one, the
   * pre-cycle draft is returned once, then null. */
  function historyNext(): string | null {
    if (historyPos < 0) return null
    historyPos -= 1
    if (historyPos < 0) {
      const draft = historyDraft
      historyDraft = null
      return draft
    }
    return history.value[historyPos] ?? null
  }

  return {
    draftKey,
    draftSaved,
    sql,
    executing,
    results,
    error,
    queryID,
    history,
    execute,
    explain,
    cancel,
    historyPrev,
    historyNext,
    resetHistoryCycle,
  }
}

export type Console = ReturnType<typeof useConsole>
