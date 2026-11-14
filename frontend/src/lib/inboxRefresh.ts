import * as InboxService from '../../bindings/github.com/ousiassllc/hitomado/internal/app/inboxservice'
import type { useAccountsStore } from '../stores/accounts'
import { reportRefreshResults } from '../stores/inboxFailures'
import type { useUiStore } from '../stores/ui'
import { type AppError, list } from './api'
import { common } from './messages'

/** 手動更新: 差分同期を今すぐ実行する（再取得は呼び出し側）。要再接続・オフラインは共通状態へ、それ以外の失敗は再試行つきのアラート。 */
export async function refreshAccounts(
  accountIds: readonly string[],
  api: <T>(p: PromiseLike<T>) => Promise<T>,
  accounts: ReturnType<typeof useAccountsStore>,
  ui: ReturnType<typeof useUiStore>,
  retry: () => void,
): Promise<void> {
  try {
    const res = await api(InboxService.Refresh([...accountIds]))
    reportRefreshResults(list(res.results), {
      markNeedsReauth: accounts.markNeedsReauth,
      setOffline: () => ui.setOnline(false),
      nameOf: (id) => accounts.byId(id)?.displayName ?? id,
      alert: (m) => ui.pushAlert(m),
    })
  } catch (e) {
    if ((e as AppError).code !== 'OFFLINE') ui.pushAlert(common.op.loadFailed, retry)
  }
}
