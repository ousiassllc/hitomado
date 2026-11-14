import { computed } from 'vue'
import { unreadTotalOf } from '../lib/inboxViews'
import { useInboxStore } from '../stores/inbox'

/** 受信箱の未読数（サイドバーの「受信箱」・下タブのバッジ）。 */
export function useUnreadTotal() {
  const inbox = useInboxStore()
  return computed(() => unreadTotalOf(inbox.unreadCounts, inbox.filter.accountIds))
}
