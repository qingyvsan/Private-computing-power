export function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i]
}

export function formatPercent(value: number): string {
  return (value * 100).toFixed(1) + '%'
}

export function formatTime(ts: number): string {
  if (!ts) return '-'
  // protobuf Timestamp is in seconds; if value looks like seconds, convert to ms
  const d = new Date(ts < 1e12 ? ts * 1000 : ts)
  return d.toLocaleString('zh-CN')
}

export function formatDuration(seconds: number): string {
  if (!seconds) return '-'
  const s = Math.floor(seconds)
  if (s < 60) return s + 's'
  const minutes = Math.floor(s / 60)
  if (minutes < 60) return minutes + 'm ' + (s % 60) + 's'
  const hours = Math.floor(minutes / 60)
  return hours + 'h ' + (minutes % 60) + 'm'
}

export function statusColor(status: string): string {
  switch (status) {
    case 'online':
    case 'running':
    case 'completed':
      return '#67c23a'
    case 'busy':
      return '#e6a23c'
    case 'offline':
    case 'failed':
    case 'cancelled':
    case 'error':
      return '#f56c6c'
    case 'pending':
    case 'assigned':
      return '#409eff'
    default:
      return '#909399'
  }
}

export function statusType(status: string): 'success' | 'warning' | 'danger' | 'info' | 'primary' {
  switch (status) {
    case 'online':
    case 'completed':
      return 'success'
    case 'busy':
    case 'pending':
    case 'assigned':
      return 'primary'
    case 'offline':
    case 'failed':
    case 'cancelled':
    case 'error':
      return 'danger'
    default:
      return 'info'
  }
}