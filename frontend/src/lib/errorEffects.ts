// API エラーが UI の共通状態に及ぼす影響（オフライン表示・要再接続）の判定。ストア間の循環参照を避けるため純関数にする。
import type { AppError } from './api'

export interface ErrorEffects {
  setOnline(online: boolean): void
  markNeedsReauth(accountId: string): void
}

/** `details.failedRefs`（複数 ref 操作で失敗した ref）を取り出す。 */
export function failedRefsOf(e: AppError): { accountId: string; threadId: string }[] {
  const v = e.details?.failedRefs
  if (!Array.isArray(v)) return []
  return v.filter(
    (r): r is { accountId: string; threadId: string } =>
      typeof r === 'object' &&
      r !== null &&
      typeof r.accountId === 'string' &&
      typeof r.threadId === 'string',
  )
}

/** OFFLINE → オフライン表示、NEEDS_REAUTH → 該当アカウントを要再接続にする。 */
export function applyErrorEffects(e: AppError, fx: ErrorEffects): void {
  if (e.code === 'OFFLINE') {
    fx.setOnline(false)
    return
  }
  if (e.code === 'NEEDS_REAUTH') {
    const ids = new Set<string>()
    if (e.accountId) ids.add(e.accountId)
    for (const r of failedRefsOf(e)) ids.add(r.accountId)
    for (const id of ids) fx.markNeedsReauth(id)
  }
}
