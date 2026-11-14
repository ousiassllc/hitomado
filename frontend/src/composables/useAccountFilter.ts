import { computed } from 'vue'
import { nextAccountIds } from '../lib/accountSelect'
import { useInboxStore } from '../stores/inbox'

/**
 * アカウントの絞り込み（サイドバーのリストとスマホのチップで共通）。見る場所・未読のみとは別の軸で、組み合わせて効く
 * （setFilter は未読のみを保つ）。選ぶとサーバーへ保存し、一覧を取り直す。
 */
export function useAccountFilter() {
  const inbox = useInboxStore()
  const selectedIds = computed(() => new Set(inbox.filter.accountIds))
  const allSelected = computed(() => selectedIds.value.size === 0)

  function toggle(id: string, multi: boolean): void {
    void inbox.setFilter({
      accountIds: nextAccountIds(inbox.filter.accountIds, id, multi),
      unreadOnly: inbox.filter.unreadOnly,
    })
  }

  function selectAll(): void {
    void inbox.setFilter({ accountIds: [], unreadOnly: inbox.filter.unreadOnly })
  }

  return { selectedIds, allSelected, toggle, selectAll }
}
