// 新しい版のバナー（設定の「更新」まわり）。自動アップデート中（Windows 版）は進みを、それ以外はダウンロードページのリンクを出す。
import type { Banner } from '../stores/ui'
import type { UpdateStatusPayload } from './events'
import { settings as msg } from './messages'

export const UPDATE_BANNER_KEY = 'update'

export interface UpdateBannerInput {
  /** 更新確認の結果（新しい版があるか・ダウンロードページ）。 */
  info?: { available: boolean; version?: string; url?: string }
  /** 自動アップデートの進み。 */
  status: UpdateStatusPayload
  /** 自動アップデートを使える（Windows 版で設定がオン）か。 */
  autoUpdate: boolean
  openExternal: (url: string) => void
  restart: () => void
}

/** 出すバナー。出さないときは undefined。 */
export function buildUpdateBanner(i: UpdateBannerInput): Banner | undefined {
  const { info, status } = i
  const key = UPDATE_BANNER_KEY
  const link = (url?: string) =>
    url ? { label: msg.update.bannerAction, run: () => i.openExternal(url) } : undefined

  if (i.autoUpdate && status.stage !== 'none') {
    const version = status.version ?? info?.version ?? ''
    switch (status.stage) {
      case 'ready':
        return {
          key,
          kind: 'update',
          message: [msg.update.ready(version), status.message].filter(Boolean).join(' '),
          action: { label: msg.update.restart, run: i.restart },
        }
      case 'downloading':
        return { key, kind: 'update', message: msg.update.downloading(version) }
      default:
        // 失敗（通信・ハッシュ不一致・インストーラー）。今の版のまま使え、リンクからは手動で入れ替えられる。
        return {
          key,
          kind: 'update',
          message: status.message || msg.update.banner(version),
          action: link(info?.url),
        }
    }
  }
  if (!info?.available || !info.version) return undefined
  return { key, kind: 'update', message: msg.update.banner(info.version), action: link(info.url) }
}
