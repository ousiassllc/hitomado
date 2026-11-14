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
import { useUiStore } from '../stores/ui'

/**
 * 今の画面（1 画面ずつのレイアウトで何を見せるか）と、戻るの実行。状態は inbox・compose・ui の既存の状態から導くだけで、
 * 遷移専用の状態は持たない（幅の切り替えで選択中のスレッド・返信の下書きが消えない）。
 */
export function useNavigation() {
  const inbox = useInboxStore()
  const compose = useComposeStore()
  const ui = useUiStore()

  // DOM（Reka UI のダイアログ・メニュー）は反応性が無いので、overlayOpen は goBack の時点で調べる。
  const state = computed<Omit<NavState, 'overlayOpen'>>(() => {
    const s = compose.session
    const sel = inbox.selectedRef
    // 返信・転送は、開いているスレッドへのものだけが「返信画面」（別スレッドの下書きは裏に残るだけ）
    const forThread = s !== undefined && sel !== undefined && compose.isOpenForThread(sel)
    return {
      agentOpen: ui.agentOpen,
      composeMode: s && (s.mode === 'new' || forThread) ? (s.mode as ComposeMode) : undefined,
      threadOpen: sel !== undefined,
      searching: inbox.mode === 'search',
      viewOpen: inbox.view !== 'inbox',
    }
  })

  const screen = computed(() => screenOf(state.value))

  /**
   * 戻る。一つ前の画面へ戻したら true。戻る先が無ければ false（アプリはバックグラウンドへ）。
   * オーバーレイ（Reka UI のダイアログ・メニュー）は、Esc と同じ経路で閉じる。
   */
  function goBack(): boolean {
    const overlayOpen = ui.anyDialogOpen() || hasOpenDialog(document) || hasOpenPopup(document)
    // 設定の AI のプロバイダのページは、ほかのダイアログ・メニューが前面に無いときだけ、一覧へ戻す（#315）。
    const settingsPageOpen =
      ui.dialogs.settings &&
      ui.settingsTab === 'ai' &&
      ui.aiProviderPage !== undefined &&
      ui.overlays.length === 1 &&
      document.querySelectorAll('[role="dialog"], [role="alertdialog"]').length <= 1 &&
      !hasOpenPopup(document)
    const action: BackAction | undefined = backActionOf({
      ...state.value,
      overlayOpen,
      settingsPageOpen,
    })
    switch (action) {
      case 'dismissOverlay':
        document.dispatchEvent(
          new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true }),
        )
        break
      case 'closeSettingsPage':
        ui.aiProviderPage = undefined
        break
      case 'closeAgent':
        ui.closeAgent()
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
      case 'closeView':
        void inbox.setView('inbox')
        break
    }
    return action !== undefined
  }

  return { screen, goBack }
}
