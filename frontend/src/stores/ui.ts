import { defineStore } from 'pinia'
import { ref } from 'vue'

export type ToastKind = 'success' | 'error' | 'info'

export interface Toast {
  id: number
  kind: ToastKind
  message: string
  /** 再試行ボタン用。指定時はエラートーストに再試行ボタンを出す */
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
  const toasts = ref<Toast[]>([])
  const banners = ref<Banner[]>([])
  const dialogs = ref<Record<DialogName, boolean>>({
    settings: false,
    addAccount: false,
    sendConfirm: false,
  })
  const settingsTab = ref('accounts')
  const online = ref(true)
  /** サイドバーのドロワー（2 ペイン・1 画面ずつのレイアウトで使う）が開いているか */
  const menuOpen = ref(false)
  let nextId = 1

  function pushToast(kind: ToastKind, message: string, retry?: () => void): number {
    const id = nextId++
    toasts.value.push({ id, kind, message, retry })
    return id
  }

  function dismissToast(id: number): void {
    toasts.value = toasts.value.filter((t) => t.id !== id)
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
  function openSettings(tab = 'accounts'): void {
    settingsTab.value = tab
    openDialog('settings')
  }

  function openMenu(): void {
    menuOpen.value = true
  }

  function closeMenu(): void {
    menuOpen.value = false
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
    toasts,
    banners,
    dialogs,
    settingsTab,
    online,
    menuOpen,
    pushToast,
    dismissToast,
    setBanner,
    clearBanner,
    openDialog,
    closeDialog,
    openSettings,
    openMenu,
    closeMenu,
    setOnline,
    anyDialogOpen,
  }
})
