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
import { useInboxStore } from '../stores/inbox'
import { useUiStore } from '../stores/ui'

const G_TIMEOUT_MS = 1500

/** グローバルキーボードショートカット（components/overview.md §4）を登録する。AppShell から 1 回だけ呼ぶ。 */
export function useShortcuts(): void {
  const inbox = useInboxStore()
  const compose = useComposeStore()
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
      void inbox.setView('inbox')
    },
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
    handlers[r.action]()
  }

  onMounted(() => window.addEventListener('keydown', onKeydown))
  onBeforeUnmount(() => {
    window.removeEventListener('keydown', onKeydown)
    clearTimeout(gTimer)
  })
}
