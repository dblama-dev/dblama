// Per-server SQL console history kept in localStorage. Each server gets
// its own list under `dbm-console-${serverKey}`, stored most-recent-first
// and capped so it can never grow without bound.

const CAP = 100

function storageKey(serverKey: string): string {
  return `dbm-console-${serverKey}`
}

/** Reads the stored history for one server, newest first. A corrupted
 * payload (or another tab's junk) counts as an empty history. */
export function loadHistory(serverKey: string): string[] {
  try {
    const raw = localStorage.getItem(storageKey(serverKey))
    if (raw === null) return []
    const parsed: unknown = JSON.parse(raw)
    return Array.isArray(parsed) ? parsed.filter((e): e is string => typeof e === 'string') : []
  } catch {
    return []
  }
}

/** Records one executed statement: a consecutive repeat of the most
 * recent entry is a no-op, whitespace-only sql is ignored, and the list
 * is trimmed to the CAP newest entries. */
export function pushHistory(serverKey: string, sql: string): void {
  const entry = sql.trim()
  if (entry === '') return
  const history = loadHistory(serverKey)
  if (history[0] === entry) return
  history.unshift(entry)
  try { localStorage.setItem(storageKey(serverKey), JSON.stringify(history.slice(0, CAP))) }
  catch { /* Storage failure must never turn a successful SQL operation into an error. */ }
}
