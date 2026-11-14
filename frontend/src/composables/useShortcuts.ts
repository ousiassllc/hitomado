import { onBeforeUnmount, onMounted } from 'vue'
import {
  hasOpenDialog,
  hasOpenPopup,
  keyInfoOf,
  resolveShortcut,
  SEARCH_INPUT_ATTR,
  type ShortcutAction,
} from '../lib/shortcuts'
import { useComposeStore } from '../stores/compose'
import { useDraftsStore } from '../stores/drafts'
import { useInboxStore } from '../stores/inbox'
import { useUiStore } from '../stores/ui'

const G_TIMEOUT_MS = 1500

/** グローバルキーボードショートカット（components/overview.md §4）を登録する。AppShell から 1 回だけ呼ぶ。 */
export function useShortcuts(): void {
  const inbox = useInboxStore()
  const compose = useComposeStore()
  const drafts = useDraftsStore()
  const ui = useUiStore()
  let pendingG = false
  let gTimer: ReturnType<typeof setTimeout> | undefined

  function setPendingG(v: boolean): void {
    pendingG = v
    clearTimeout(gTimer)
    if (v) gTimer = setTimeout(() => setPendingG(false), G_TIMEOUT_MS)
  }

  function openReply(mode: 'reply' | 'reply_all' | 'forward'): void {
    const ref = inbox.selectedRef
    if (ref) void compose.open({ mode, ref })
  }

  function closeOrClear(): void {
    if (hasOpenPopup(document)) return // ポップアップ側（Reka UI）が閉じる
    if (ui.agentOpen) {
      ui.closeAgent()
      return
    }
    if (inbox.mode === 'search') void inbox.clearSearch()
    else if (inbox.selectedKey) inbox.select(undefined)
  }

  /** j/k: スレッドを開いているときは開くスレッドを切り替え、そうでなければカーソルだけ動かす。 */
  function move(delta: number): void {
    const reading = inbox.selectedKey !== undefined
    inbox.moveCursor(delta)
    if (reading) inbox.openCursor()
  }

  const handlers: Record<ShortcutAction, () => void> = {
    next: () => move(1),
    prev: () => move(-1),
    open: () => inbox.openCursor(),
    escape: closeOrClear,
    archive: () => void inbox.archive(inbox.targetRefs),
    trash: () => void inbox.trash(inbox.targetRefs),
    markRead: () => void inbox.markRead(inbox.targetRefs, true),
    markUnread: () => void inbox.markRead(inbox.targetRefs, false),
    reply: () => openReply('reply'),
    replyAll: () => openReply('reply_all'),
    forward: () => openReply('forward'),
    compose: () => void compose.open({ mode: 'new' }),
    search: () => document.querySelector<HTMLElement>(`[${SEARCH_INPUT_ATTR}]`)?.focus(),
    goInbox: () => {
      ui.closeAgent()
      drafts.hide()
      void inbox.setView('inbox')
    },
  }

  /**
   * 下書きの一覧を出している間は、見えていない受信箱の行を対象にする操作（移動・開く・アーカイブなど）を受け付けない。
   * ただし、一覧から返信の下書きを開いて、そのスレッドが見えているとき（複数選択なし・カーソルが開いているスレッド）は、
   * そのスレッドへのアーカイブ・削除・既読・未読は受け付ける。
   */
  const INBOX_LIST_ACTIONS: ReadonlySet<ShortcutAction> = new Set([
    'next',
    'prev',
    'open',
    'archive',
    'trash',
    'markRead',
    'markUnread',
  ])

  const THREAD_ACTIONS: ReadonlySet<ShortcutAction> = new Set([
    'archive',
    'trash',
    'markRead',
    'markUnread',
  ])

  /** 下書きの一覧の間に、見えている（開いている）スレッドだけが操作の対象か。 */
  function targetsOpenThread(): boolean {
    return (
      inbox.selectedKey !== undefined &&
      inbox.checkedKeys.length === 0 &&
      inbox.cursorKey === inbox.selectedKey
    )
  }

  function onKeydown(ev: KeyboardEvent): void {
    if (ev.defaultPrevented || ev.isComposing) return
    const r = resolveShortcut(keyInfoOf(ev), {
      dialogOpen: ui.anyDialogOpen() || hasOpenDialog(document),
      pendingG,
    })
    setPendingG(r.pendingG)
    if (!r.action) return
    // Esc は何も閉じるものが無いときに他の処理（ブラウザ既定）を妨げない
    if (r.action !== 'escape') ev.preventDefault()
    if (drafts.active && INBOX_LIST_ACTIONS.has(r.action)) {
      if (!THREAD_ACTIONS.has(r.action) || !targetsOpenThread()) return
    }
    handlers[r.action]()
  }

  onMounted(() => window.addEventListener('keydown', onKeydown))
  onBeforeUnmount(() => {
    window.removeEventListener('keydown', onKeydown)
    clearTimeout(gTimer)
  })
}
