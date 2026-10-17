/** Display helpers. Kept in one place so every column formats identically. */

const UNITS = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']

export function formatBytes(bytes: number, digits = 1): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 B'
  let value = bytes
  let unit = 0
  while (value >= 1024 && unit < UNITS.length - 1) {
    value /= 1024
    unit += 1
  }
  const fixed = unit === 0 ? 0 : digits
  return `${value.toFixed(fixed)} ${UNITS[unit]}`
}

export function formatSpeed(bytesPerSecond: number): string {
  if (!Number.isFinite(bytesPerSecond) || bytesPerSecond <= 0) return '—'
  return `${formatBytes(bytesPerSecond, 1)}/s`
}

/**
 * Elapsed/remaining times. FlashGet showed h:mm:ss; we keep that but drop the
 * leading unit when it is zero so short downloads stay compact.
 */
export function formatDuration(ms: number): string {
  if (!Number.isFinite(ms) || ms <= 0) return '—'

  const total = Math.floor(ms / 1000)
  const hours = Math.floor(total / 3600)
  const minutes = Math.floor((total % 3600) / 60)
  const seconds = total % 60

  const pad = (n: number) => String(n).padStart(2, '0')
  return hours > 0 ? `${hours}:${pad(minutes)}:${pad(seconds)}` : `${minutes}:${pad(seconds)}`
}

/** Remaining time is meaningless when nothing is moving. */
export function formatRemaining(ms: number, speed: number): string {
  if (speed <= 0) return '未知'
  return formatDuration(ms)
}

export function percent(done: number, size: number): number {
  if (!size || size <= 0) return 0
  const pct = (done / size) * 100
  return Math.max(0, Math.min(100, pct))
}

export function formatPercent(done: number, size: number): string {
  if (!size || size <= 0) return '—'
  return `${percent(done, size).toFixed(1)}%`
}

/** A stable hue per jet index, so the block map and jet list agree. */
const JET_HUES = [205, 145, 35, 275, 0, 190, 95, 320]

export function jetColor(jet: number, alpha = 1): string {
  if (jet < 0) return `rgba(150, 158, 170, ${alpha})`
  const hue = JET_HUES[jet % JET_HUES.length]
  return `hsl(${hue} 72% 55% / ${alpha})`
}
