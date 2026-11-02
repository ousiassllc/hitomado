// バナーの選択（優先度順に最大 2 件）。純関数。
import type { Banner, BannerKind } from '../stores/ui'
import { common } from './messages'

export const MAX_BANNERS = 2

/** 小さいほど優先（オフライン → 要再接続 → 更新あり → 同期エラー）。 */
const PRIORITY: Record<BannerKind, number> = {
  offline: 0,
  needs_reauth: 1,
  update: 2,
  sync_error: 3,
}

/** key が重複したら先勝ち。優先度が同じなら入力順を保つ。 */
export function selectBanners(banners: Banner[], max: number = MAX_BANNERS): Banner[] {
  const seen = new Set<string>()
  const unique = banners.filter((b) => !seen.has(b.key) && seen.add(b.key))
  return unique
    .map((b, i) => ({ b, i }))
    .sort((x, y) => PRIORITY[x.b.kind] - PRIORITY[y.b.kind] || x.i - y.i)
    .slice(0, max)
    .map((x) => x.b)
}

export interface BannerSources {
  online: boolean
  accounts: { id: string; displayName: string }[]
  statuses: Record<string, string>
  statusMessages: Record<string, string>
  /** uiStore に積まれたバナー（更新あり等） */
  extra: Banner[]
  reconnect: (accountId: string) => void
}

/**
 * 表示するバナーの全候補を作る: オフライン・要再接続（アカウントごと）・同期エラー（アカウントごと）と、
 * uiStore のバナー。最終的な絞り込みは selectBanners。
 */
export function deriveBanners(s: BannerSources): Banner[] {
  const out: Banner[] = []
  if (!s.online) out.push({ key: 'offline', kind: 'offline', message: common.offlineBanner })
  for (const a of s.accounts) {
    if (s.statuses[a.id] === 'needs_reauth') {
      out.push({
        key: `reauth:${a.id}`,
        kind: 'needs_reauth',
        message: common.needsReauthBanner(a.displayName),
        action: { label: common.reconnect, run: () => s.reconnect(a.id) },
      })
    } else if (s.statuses[a.id] === 'error') {
      out.push({
        key: `sync_error:${a.id}`,
        kind: 'sync_error',
        message: s.statusMessages[a.id] || common.syncErrorBanner(a.displayName),
      })
    }
  }
  return [...out, ...s.extra]
}
