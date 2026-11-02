// キーボードショートカットの判定（components/overview.md §4）。DOM に依存しない純関数。

export type ShortcutAction =
  | 'next'
  | 'prev'
  | 'open'
  | 'escape'
  | 'archive'
  | 'trash'
  | 'markRead'
  | 'markUnread'
  | 'reply'
  | 'replyAll'
  | 'forward'
  | 'compose'
  | 'search'
  | 'goInbox'

export interface KeyInfo {
  key: string
  shiftKey: boolean
  ctrlKey: boolean
  metaKey: boolean
  altKey: boolean
  /** input / textarea / select / contenteditable（エディタ）上のイベント */
  inEditable: boolean
  /** Enter が本来の動作を持つ要素（ボタン・リンク・メニュー項目など）上のイベント */
  onInteractive: boolean
}

export interface ShortcutContext {
  /** ダイアログ表示中 */
  dialogOpen: boolean
  /** `g` を押した直後（`g` → `i` の 2 打鍵待ち） */
  pendingG: boolean
}

export interface ShortcutResult {
  action?: ShortcutAction
  /** 次のキー入力時の `pendingG` */
  pendingG: boolean
}

/** 検索窓の識別用属性（`/` でのフォーカス先）。 */
export const SEARCH_INPUT_ATTR = 'data-search-input'

const SIMPLE: Record<string, ShortcutAction> = {
  j: 'next',
  k: 'prev',
  o: 'open',
  e: 'archive',
  '#': 'trash',
  r: 'reply',
  a: 'replyAll',
  f: 'forward',
  c: 'compose',
  '/': 'search',
}

/**
 * キー入力をアクションに変換する。入力要素・ダイアログ表示中・Ctrl/Alt/Meta 併用は常に無効。
 * 単独キーのうち `Shift+i`／`Shift+u` と `#`（Shift を伴う記号）だけ Shift を許す。
 */
export function resolveShortcut(ev: KeyInfo, ctx: ShortcutContext): ShortcutResult {
  const none: ShortcutResult = { pendingG: false }
  if (ev.inEditable || ctx.dialogOpen || ev.ctrlKey || ev.metaKey || ev.altKey) return none

  if (ctx.pendingG && ev.key === 'i' && !ev.shiftKey) return { action: 'goInbox', pendingG: false }

  if (ev.key === 'Escape') return { action: 'escape', pendingG: false }
  if (ev.key === 'Enter')
    return ev.onInteractive || ev.shiftKey ? none : { action: 'open', pendingG: false }
  if (ev.key === 'I' && ev.shiftKey) return { action: 'markRead', pendingG: false }
  if (ev.key === 'U' && ev.shiftKey) return { action: 'markUnread', pendingG: false }
  if (ev.key === 'g' && !ev.shiftKey) return { action: undefined, pendingG: true }
  if (ev.key === '#') return { action: 'trash', pendingG: false }

  const action = SIMPLE[ev.key]
  if (action && !ev.shiftKey) return { action, pendingG: false }
  return none
}

const EDITABLE_TAGS = new Set(['INPUT', 'TEXTAREA', 'SELECT'])
const INTERACTIVE_TAGS = new Set(['BUTTON', 'A', 'SUMMARY'])
const INTERACTIVE_ROLES = new Set(['button', 'menuitem', 'tab', 'switch', 'checkbox', 'combobox'])

/** フォーカス要素が入力要素（contenteditable・エディタ含む）か。 */
export function isEditableElement(el: EventTarget | null): boolean {
  if (!(el instanceof HTMLElement)) return false
  if (EDITABLE_TAGS.has(el.tagName) || el.isContentEditable) return true
  return el.closest('[contenteditable="true"], [contenteditable=""], .ProseMirror') !== null
}

/** Enter／o を奪ってはいけない要素か（ボタンの押下など）。 */
export function isInteractiveElement(el: EventTarget | null): boolean {
  if (!(el instanceof HTMLElement)) return false
  if (INTERACTIVE_TAGS.has(el.tagName)) return true
  const role = el.getAttribute('role')
  return role !== null && INTERACTIVE_ROLES.has(role)
}

export function keyInfoOf(ev: KeyboardEvent): KeyInfo {
  return {
    key: ev.key,
    shiftKey: ev.shiftKey,
    ctrlKey: ev.ctrlKey,
    metaKey: ev.metaKey,
    altKey: ev.altKey,
    inEditable: isEditableElement(ev.target),
    onInteractive: isInteractiveElement(ev.target),
  }
}

/** Esc で最初に閉じるべき Reka UI のポップアップ（メニュー・セレクト）が開いているか。 */
export function hasOpenPopup(doc: Document): boolean {
  return doc.querySelector('[data-reka-popper-content-wrapper]') !== null
}

/** uiStore に載らないダイアログ（作成画面内の確認など。Reka UI の DialogContent は role="dialog"）が表示中か。 */
export function hasOpenDialog(doc: Document): boolean {
  return doc.querySelector('[role="dialog"], [role="alertdialog"]') !== null
}
