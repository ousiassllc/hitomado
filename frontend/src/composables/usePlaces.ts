import { computed } from 'vue'
import type { InboxView } from '../lib/inboxViews'
import { useDraftsStore } from '../stores/drafts'
import { useInboxStore } from '../stores/inbox'

/** 「見る場所」（サイドバー・下タブ）。受信箱・送信済み・アーカイブは inbox.view、下書きは下書きの一覧（#366）。 */
export type Place = InboxView | 'drafts'

/** 今いる場所と、場所の切り替え。下書きの一覧は受信箱の上に重ねて出すので、開いている間は下書きが今の場所。 */
export function usePlaces() {
  const inbox = useInboxStore()
  const drafts = useDraftsStore()

  /** 今いる場所。検索中は全メールが対象なので、どの場所も強調しない（undefined）。 */
  const current = computed<Place | undefined>(() => {
    if (drafts.active) return 'drafts'
    return inbox.mode === 'search' ? undefined : inbox.view
  })

  function show(place: Place): void {
    if (place === 'drafts') {
      drafts.show()
      return
    }
    drafts.hide()
    void inbox.setView(place)
  }

  return { current, show }
}
