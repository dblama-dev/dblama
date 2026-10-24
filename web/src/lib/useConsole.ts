import { ref } from 'vue'

import { api } from '../api/client'
import type { QueryResult } from '../api/types'
import { loadHistory, pushHistory } from './consoleHistory'

/** db getter and localStorage history key of the console's server. */
export interface ConsoleScope {
  db: () => string
  serverKey: string
}

/** All state and actions of the SQL console except the CodeMirror
 * wiring: execution (all / selection / EXPLAIN), cancel, results and the
 * per-server history cycle behind Alt+Up/Down. Extracted from
 * ConsoleView so the orchestration is testable without the editor. */
export function useConsole(scope: ConsoleScope) {
  const sql = ref('')
  const executing = ref(false)
  const results = ref<QueryResult[] | null>(null)
  const error = ref('')
  /** queryID of the run currently executing (or the last one). */
  const queryID = ref('')
  const history = ref<string[]>(loadHistory(scope.serverKey))
  /** Position inside history while cycling; -1 means "not cycling". */
  let historyPos = -1

  function resetHistoryPos(): void {
    historyPos = -1
  }

  /** Runs one SQL batch: queryID per run, backend defaults for maxRows
   * (1000) and timeoutSec (60). On success the executed sql is recorded
   * in the server history; on failure the message is stored and the old
   * results are dropped (the backend sends no partial results). */
  async function run(text: string): Promise<boolean> {
    const trimmed = text.trim()
    if (trimmed === '' || executing.value) return false
    executing.value = true
    error.value = ''
    results.value = null
    queryID.value = crypto.randomUUID()
    try {
      const payload = await api<{ results: QueryResult[] }>(
        'POST',
        `/api/databases/${encodeURIComponent(scope.db())}/query`,
        { sql: trimmed, queryID: queryID.value },
      )
      results.value = payload.results ?? []
      pushHistory(scope.serverKey, trimmed)
      history.value = loadHistory(scope.serverKey)
      resetHistoryPos()
      return true
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
      return false
    } finally {
      executing.value = false
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
    error.value = ''
    results.value = null
    try {
      const payload = await api<{ result: QueryResult | null }>('POST', '/api/query', {
        db: scope.db(),
        sql: trimmed,
      })
      results.value = payload.result === null ? [] : [payload.result]
      return true
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
      return false
    } finally {
      executing.value = false
    }
  }

  /** Cancels the current run; the pending execute() rejects with the
   * driver's cancel message, which lands in `error`. */
  async function cancel(): Promise<void> {
    if (queryID.value === '') return
    await api('POST', `/api/query/${queryID.value}/cancel`)
  }

  /** Alt+Up: the next older history entry (starting at the most recent),
   * or null when the history is exhausted. */
  function historyPrev(): string | null {
    if (history.value.length === 0) return null
    historyPos = Math.min(historyPos + 1, history.value.length - 1)
    return history.value[historyPos] ?? null
  }

  /** Alt+Down: the next newer entry, or null once back at the start. */
  function historyNext(): string | null {
    if (historyPos < 0) return null
    historyPos -= 1
    return historyPos < 0 ? null : (history.value[historyPos] ?? null)
  }

  return {
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
  }
}

export type Console = ReturnType<typeof useConsole>
