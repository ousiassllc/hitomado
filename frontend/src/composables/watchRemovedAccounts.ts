// 削除されたアカウントが保存済みフィルタに残らないようにする。
import { type Ref, watch } from 'vue'
import type { InboxFilter } from '../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'

export function watchRemovedAccounts(
  accounts: { accounts: { id: string }[] },
  filter: Ref<{ accountIds: string[]; unreadOnly: boolean }>,
  setFilter: (f: InboxFilter) => Promise<void>,
): () => void {
  return watch(
    () => accounts.accounts.map((a) => a.id).join(','),
    () => {
      const ids = new Set(accounts.accounts.map((a) => a.id))
      if (accounts.accounts.length > 0 && filter.value.accountIds.some((id) => !ids.has(id))) {
        void setFilter({
          accountIds: filter.value.accountIds.filter((id) => ids.has(id)),
          unreadOnly: filter.value.unreadOnly,
        })
      }
    },
  )
}
