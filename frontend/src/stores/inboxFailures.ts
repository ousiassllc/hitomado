// ページ取得の部分失敗（アカウントごと）の整形。
import type {
  AccountFailure,
  RefreshResult,
} from '../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import { isAppErrorLike, list } from '../lib/api'
import { common } from '../lib/messages'

export interface Failure {
  accountId: string
  code: string
  message: string
}

export function toFailures(fs: AccountFailure[] | null | undefined): Failure[] {
  return list(fs).map((f) => {
    const e = isAppErrorLike(f.error) ? f.error : undefined
    return {
      accountId: f.accountId,
      code: e?.code ?? 'INTERNAL',
      message: e?.message ?? common.unexpectedError,
    }
  })
}

export interface RefreshReportDeps {
  markNeedsReauth: (accountId: string) => void
  setOffline: () => void
  nameOf: (accountId: string) => string
  alert: (message: string) => void
}

/** 手動更新のアカウントごとの結果を反映する: 要再接続・オフラインは共通状態へ、それ以外はエラートースト。 */
export function reportRefreshResults(results: RefreshResult[], d: RefreshReportDeps): void {
  for (const r of results) {
    if (r.ok) continue
    const code = isAppErrorLike(r.error) ? r.error.code : undefined
    if (code === 'NEEDS_REAUTH') d.markNeedsReauth(r.accountId)
    else if (code === 'OFFLINE') d.setOffline()
    else d.alert(common.op.refreshFailed(d.nameOf(r.accountId)))
  }
}
