// 日時・サイズの整形（日本語）。純関数。

function pad2(n: number): string {
  return String(n).padStart(2, '0')
}

/**
 * 一覧用の日時。今日は時刻（HH:mm）、今年は月日（M月D日）、それ以外は年月日（YYYY年M月D日）。
 * `ms` は Unix ミリ秒。日付の境界は実行環境のローカルタイムゾーンで判定する。
 */
export function formatDate(ms: number, now: Date = new Date()): string {
  const d = new Date(ms)
  if (d.getFullYear() === now.getFullYear()) {
    if (d.getMonth() === now.getMonth() && d.getDate() === now.getDate()) {
      return `${pad2(d.getHours())}:${pad2(d.getMinutes())}`
    }
    return `${d.getMonth() + 1}月${d.getDate()}日`
  }
  return `${d.getFullYear()}年${d.getMonth() + 1}月${d.getDate()}日`
}

/** 詳細用の日時（YYYY年M月D日 HH:mm）。 */
export function formatDateTime(ms: number): string {
  const d = new Date(ms)
  return `${d.getFullYear()}年${d.getMonth() + 1}月${d.getDate()}日 ${pad2(d.getHours())}:${pad2(d.getMinutes())}`
}

/** バイト数を B / KB / MB / GB に整形する（1024 進数、小数 1 桁。B は整数）。 */
export function formatSize(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes < 0) return '0 B'
  if (bytes < 1024) return `${Math.round(bytes)} B`
  const units = ['KB', 'MB', 'GB']
  let v = bytes / 1024
  let i = 0
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(1)} ${units[i]}`
}
