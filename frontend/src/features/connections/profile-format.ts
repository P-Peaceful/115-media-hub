/** Formats storage values using binary units so account and capacity views agree. */
export function formatBytes(value: number | null | undefined) {
  if (value == null || !Number.isFinite(value) || value < 0) return '未知'
  if (value < 1024) return `${value} B`
  const units = ['KiB', 'MiB', 'GiB', 'TiB', 'PiB']
  let size = value
  let unit = -1
  do {
    size /= 1024
    unit += 1
  } while (size >= 1024 && unit < units.length - 1)
  return `${size.toFixed(1)} ${units[unit]}`
}
