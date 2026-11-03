// 画面遷移の判断（純粋関数）。「今どの画面か」と「戻るで何をするか」は、すでにあるストアの状態から導く
// （遷移専用の状態は持たない）。そのため、幅を変えて 3 ペイン⇔1 画面ずつを切り替えても、
// 開いているスレッド・返信の下書きは失われない。

export type ComposeMode = 'new' | 'reply' | 'reply_all' | 'forward'

/** 1 画面ずつのレイアウトで表示する画面。list = 一覧（ルート）、reply = 開いているスレッドへの返信・転送。 */
export type Screen = 'list' | 'thread' | 'reply' | 'compose'

export interface NavState {
  /** ダイアログ・メニューなどのオーバーレイが開いている */
  overlayOpen: boolean
  /** サイドバー（ドロワー）が開いている。ドロワーを使わない幅では false を渡す */
  menuOpen: boolean
  /** 開いている作成セッションの種類。返信・転送は「開いているスレッドへのもの」だけを渡す */
  composeMode?: ComposeMode
  threadOpen: boolean
  /** 検索結果を表示中 */
  searching: boolean
}

export function screenOf(s: Pick<NavState, 'composeMode' | 'threadOpen'>): Screen {
  if (s.composeMode === 'new') return 'compose'
  if (s.composeMode !== undefined) return 'reply'
  return s.threadOpen ? 'thread' : 'list'
}

/** 戻るで行う 1 段階の操作。 */
export type BackAction =
  | 'dismissOverlay'
  | 'closeMenu'
  | 'closeCompose'
  | 'closeThread'
  | 'clearSearch'

/** 戻る操作の内容。戻る先が無い（一覧＝ルート）ときは undefined（アプリ側はバックグラウンドへ送る）。 */
export function backActionOf(s: NavState): BackAction | undefined {
  if (s.overlayOpen) return 'dismissOverlay'
  if (s.menuOpen) return 'closeMenu'
  const screen = screenOf(s)
  if (screen === 'reply' || screen === 'compose') return 'closeCompose'
  if (screen === 'thread') return 'closeThread'
  return s.searching ? 'clearSearch' : undefined
}
