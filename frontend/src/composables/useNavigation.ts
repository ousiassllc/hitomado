import { computed } from 'vue'
import {
  type BackAction,
  backActionOf,
  type ComposeMode,
  type NavState,
  screenOf,
} from '../lib/navigation'
import { hasOpenDialog, hasOpenPopup } from '../lib/shortcuts'
import { useComposeStore } from '../stores/compose'
import { useInboxStore } from '../stores/inbox'
import { useLayoutStore } from '../stores/layout'
import { useUiStore } from '../stores/ui'

/**
 * 今の画面（1 画面ずつのレイアウトで何を見せるか）と、戻るの実行。状態は inbox・compose・ui の既存の状態から導くだけで、
 * 遷移専用の状態は持たない（幅の切り替えで選択中のスレッド・返信の下書きが消えない）。
 */
export function useNavigation() {
  const inbox = useInboxStore()
  const compose = useComposeStore()
  const ui = useUiStore()
  const layout = useLayoutStore()

  // DOM（Reka UI のダイアログ・メニュー）は反応性が無いので、overlayOpen は goBack の時点で調べる。
  const state = computed<Omit<NavState, 'overlayOpen'>>(() => {
    const s = compose.session
    const sel = inbox.selectedRef
    // 返信・転送は、開いているスレッドへのものだけが「返信画面」（別スレッドの下書きは裏に残るだけ）
    const forThread = s !== undefined && sel !== undefined && compose.isOpenForThread(sel)
    return {
      menuOpen: ui.menuOpen && layout.hasDrawer,
      composeMode: s && (s.mode === 'new' || forThread) ? (s.mode as ComposeMode) : undefined,
      threadOpen: sel !== undefined,
      searching: inbox.mode === 'search',
    }
  })

  const screen = computed(() => screenOf(state.value))

  /**
   * 戻る。一つ前の画面へ戻したら true。戻る先が無ければ false（アプリはバックグラウンドへ）。
   * オーバーレイ（Reka UI のダイアログ・メニュー）は、Esc と同じ経路で閉じる。
   */
  function goBack(): boolean {
    const overlayOpen = ui.anyDialogOpen() || hasOpenDialog(document) || hasOpenPopup(document)
    const action: BackAction | undefined = backActionOf({ ...state.value, overlayOpen })
    switch (action) {
      case 'dismissOverlay':
        document.dispatchEvent(
          new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true }),
        )
        break
      case 'closeMenu':
        ui.closeMenu()
        break
      case 'closeCompose':
        void compose.close()
        break
      case 'closeThread':
        inbox.select(undefined)
        break
      case 'clearSearch':
        void inbox.clearSearch()
        break
    }
    return action !== undefined
  }

  return { screen, goBack }
}
