// Byte-size units for formatBytes; each step is 1024 of the previous.
const UNITS = ['B', 'KB', 'MB', 'GB'] as const

/** formatBytes renders a byte count for humans: plain bytes below 1 KB,
 * then exactly one decimal place in KB/MB/GB (1572864 → "1.5 MB"). */
export function formatBytes(n: number): string {
  if (n < 1024) {
    return `${Math.max(0, Math.floor(n))} B`
  }
  let value = n
  let unit = 0
  while (value >= 1024 && unit < UNITS.length - 1) {
    value /= 1024
    unit++
  }
  return `${value.toFixed(1)} ${UNITS[unit]}`
}
