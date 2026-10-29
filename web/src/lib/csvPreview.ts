/** Bounded RFC-style CSV preview matching encoding/csv (double-quote enclosure).
 * A cut byte window may end within a record; that unfinished tail is omitted.
 */
export function parseCsvPreview(text: string, separator = ',', enclosure = '"', inputTruncated = false): { rows: string[][]; truncated: boolean } {
  if (![',', ';', '\t'].includes(separator)) throw new Error('Unsupported CSV separator')
  if (enclosure !== '' && enclosure !== '"') throw new Error('Unsupported CSV enclosure')
  const rows: string[][] = []
  let record: string[] = []
  let field = ''
  let quoted = false
  let closed = false
  let started = false
  let width: number | undefined
  // Go encoding/csv normalizes CRLF inside quoted fields too.
  const source = text.replace(/\r\n/g, '\n')
  function finishRecord(): void {
    record.push(field)
    if (record.length > 200) throw new Error('CSV preview supports at most 200 source fields')
    if (width !== undefined && record.length !== width) throw new Error(`CSV record ${rows.length + 1}: inconsistent fields`)
    width = record.length
    rows.push(record)
    record = []; field = ''; closed = false; started = false
  }
  for (let i = 0; i < source.length; i++) {
    const char = source[i]!
    if (quoted) {
      if (char === '"') {
        if (source[i + 1] === '"') { field += '"'; i++ }
        else { quoted = false; closed = true }
      } else field += char
      continue
    }
    if (char === '\n') {
      // encoding/csv ignores empty physical lines, but retains empty fields.
      if (started || record.length || field || closed) finishRecord()
      if (rows.length >= 21) return { rows, truncated: i < source.length - 1 || inputTruncated }
    } else if (char === separator) {
      record.push(field)
      if (record.length >= 200) throw new Error('CSV preview supports at most 200 source fields')
      field = ''; closed = false; started = true
    } else if (char === '"') {
      if (field !== '' || closed) throw new Error(`CSV record ${rows.length + 1}: unexpected quote`)
      quoted = true; started = true
    } else {
      if (closed) throw new Error(`CSV record ${rows.length + 1}: characters after closing quote`)
      field += char; started = true
    }
  }
  if (!inputTruncated) {
    if (quoted) throw new Error(`CSV record ${rows.length + 1}: unterminated quote`)
    if (started || record.length || field || closed) finishRecord()
  }
  return { rows, truncated: inputTruncated }
}

export const CSV_PREVIEW_BYTES = 256 * 1024
export function readCsvPreview(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(String(reader.result ?? ''))
    reader.onerror = () => reject(reader.error ?? new Error('Cannot read CSV preview'))
    reader.readAsText(file.slice(0, CSV_PREVIEW_BYTES))
  })
}
