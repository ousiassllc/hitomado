import { defineStore } from 'pinia'
import { ref } from 'vue'

/** トースト（一瞬の結果の知らせ。1 行・同時に 1 つ）。失敗は `Alert` で伝える（ガイド O3: エラー詳細をトーストに出さない）。 */
export type ToastKind = 'success' | 'info'

/** スナックバーの操作（「元に戻す」）。戻せる結果のトーストだけが持つ。 */
export interface ToastAction {
  label: string
  run: () => void
}

export interface Toast {
  id: number
  kind: ToastKind
  message: string
  /** 持つ場合はスナックバー（戻せる結果。操作できるよう、トーストより長く出す） */
  action?: ToastAction
}

/** インラインアラートの種類（ガイドのお知らせ・成功・エラー）。注意は常設のバナー（`Banner`）が受け持つ。 */
export type AlertKind = 'error' | 'success' | 'info'

/** インラインアラート（失敗の理由や、読んでほしい補足を、閉じるまで読める形で出す。再試行できる失敗は `retry` を持つ）。 */
export interface Alert {
  id: number
  kind: AlertKind
  message: string
  retry?: () => void
}

export type BannerKind = 'offline' | 'needs_reauth' | 'update' | 'sync_error'

export interface Banner {
  /** 同一 key のバナーは 1 つにまとめる */
  key: string
  kind: BannerKind
  message: string
  /** バナー内のアクション（再接続・リンクを開く等） */
  action?: { label: string; run: () => void }
}

export type DialogName = 'settings' | 'addAccount' | 'sendConfirm'

export const useUiStore = defineStore('ui', () => {
  const theme = ref<'light' | 'dark'>('light')
  /** 同時に 1 つ。新しいものが前のものを置き換える（ガイド O3）。 */
  const toast = ref<Toast>()
  const alerts = ref<Alert[]>([])
  /** 開いているダイアログの印（開いた順）。末尾が最前面で、アラートはその中に出す（前面の外に隠れさせない）。 */
  const overlays = ref<symbol[]>([])
  const banners = ref<Banner[]>([])
  const dialogs = ref<Record<DialogName, boolean>>({
    settings: false,
    addAccount: false,
    sendConfirm: false,
  })
  const settingsTab = ref('accounts')
  /** AI 設定タブを開いた直後に始める流れ（「AI で下書き」の初回の案内から ChatGPT へ進むとき。AISettings が一度だけ受け取る）。 */
  const aiSetupIntent = ref<'chatgpt-login'>()
  /** AI 設定タブで開いているプロバイダのページ（#315）。undefined はプロバイダの一覧。戻る（Android の戻るボタン）で一覧へ戻すため、ストアに持つ。 */
  const aiProviderPage = ref<string>()
  const online = ref(true)
  /** エージェントのパネル（PC は右から本文に重ねる。スマホは下タブの上いっぱい）が開いているか。中身（エージェントモード）は別 Issue で、今は「準備中」だけ出す */
  const agentOpen = ref(false)
  let nextId = 1

  function pushToast(kind: ToastKind, message: string): number {
    const id = nextId++
    toast.value = { id, kind, message }
    return id
  }

  /** 戻せる結果（アーカイブ・ゴミ箱）を、「元に戻す」つきのスナックバーで知らせる（トーストと同じ 1 枠を使う）。 */
  function pushSnackbar(message: string, action: ToastAction): number {
    const id = nextId++
    toast.value = { id, kind: 'success', message, action }
    return id
  }

  function dismissToast(id: number): void {
    if (toast.value?.id === id) toast.value = undefined
  }

  /** 失敗を知らせる。同じ文言がすでに出ていれば、重ねずに新しいものへ置き換える。 */
  function pushAlert(message: string, retry?: () => void): number {
    return pushInline('error', message, retry)
  }

  /** 1 行に収まらない成功・お知らせ（読んでほしい補足つき）を、トーストではなくインラインアラートで出す。 */
  function pushNotice(message: string, kind: 'success' | 'info' = 'info'): number {
    return pushInline(kind, message)
  }

  function pushInline(kind: AlertKind, message: string, retry?: () => void): number {
    const id = nextId++
    alerts.value = [
      ...alerts.value.filter((a) => a.message !== message),
      { id, kind, message, retry },
    ]
    return id
  }

  function dismissAlert(id: number): void {
    alerts.value = alerts.value.filter((a) => a.id !== id)
  }

  function enterOverlay(mark: symbol): void {
    overlays.value = [...overlays.value, mark]
  }

  function leaveOverlay(mark: symbol): void {
    overlays.value = overlays.value.filter((m) => m !== mark)
  }

  /** `mark` が最前面のダイアログか。 */
  function isTopOverlay(mark: symbol): boolean {
    return overlays.value.at(-1) === mark
  }

  function setBanner(b: Banner): void {
    banners.value = [...banners.value.filter((x) => x.key !== b.key), b]
  }

  function clearBanner(key: string): void {
    banners.value = banners.value.filter((x) => x.key !== key)
  }

  function openDialog(name: DialogName): void {
    dialogs.value[name] = true
  }

  function closeDialog(name: DialogName): void {
    dialogs.value[name] = false
  }

  /** 設定ダイアログを開く（AI キー未設定の誘導などから）。tab は SettingsDialog のタブ値（既定はアカウント）。 */
  function openSettings(tab = 'accounts', aiIntent?: 'chatgpt-login'): void {
    settingsTab.value = tab
    aiSetupIntent.value = tab === 'ai' ? aiIntent : undefined
    aiProviderPage.value = undefined // 開くたびに AI はプロバイダの一覧から始める（初回の案内の入口は AISettings が ChatGPT のページへ進める）
    openDialog('settings')
  }

  /** `aiSetupIntent` を取り出して消す（同じ流れを 2 回始めない）。 */
  function takeAiSetupIntent(): 'chatgpt-login' | undefined {
    const v = aiSetupIntent.value
    aiSetupIntent.value = undefined
    return v
  }

  function openAgent(): void {
    agentOpen.value = true
  }

  function closeAgent(): void {
    agentOpen.value = false
  }

  function setOnline(v: boolean): void {
    online.value = v
  }

  /** ダイアログ表示中はショートカットを無効にするための判定 */
  function anyDialogOpen(): boolean {
    return Object.values(dialogs.value).some(Boolean)
  }

  return {
    theme,
    toast,
    alerts,
    overlays,
    banners,
    dialogs,
    settingsTab,
    aiSetupIntent,
    aiProviderPage,
    online,
    agentOpen,
    pushToast,
    pushSnackbar,
    dismissToast,
    pushAlert,
    pushNotice,
    dismissAlert,
    enterOverlay,
    leaveOverlay,
    isTopOverlay,
    setBanner,
    clearBanner,
    openDialog,
    closeDialog,
    openSettings,
    takeAiSetupIntent,
    openAgent,
    closeAgent,
    setOnline,
    anyDialogOpen,
  }
})
