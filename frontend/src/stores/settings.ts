import { defineStore } from 'pinia'
import { ref } from 'vue'
import {
  AIService,
  AppService,
  ThreadService,
} from '../../bindings/github.com/ousiassllc/hitomado/internal/app'
import type {
  AIConfig,
  AppInfo,
  UpdateInfo,
} from '../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import { call, list, parseBindingError } from '../lib/api'
import { EVENTS, onEvent, type UpdateAvailablePayload } from '../lib/events'
import { settings as msg } from '../lib/messages'
import { useUiStore } from './ui'

const UPDATE_BANNER_KEY = 'update'

/** Go は consentedProviders を nil にし得るので、画面側は常に配列として扱う。 */
function normalizeAIConfig(c: AIConfig): AIConfig {
  return { ...c, consentedProviders: list(c.consentedProviders) }
}

export const useSettingsStore = defineStore('settings', () => {
  const ui = useUiStore()
  const aiConfig = ref<AIConfig>()
  const appInfo = ref<AppInfo>()
  const update = ref<UpdateInfo>()
  let started = false
  let stopUpdateEvents: (() => void) | undefined

  /** 外部リンク（README・更新ページ）は ThreadService.OpenExternal のみで開く。失敗はトーストで知らせる。 */
  async function openExternal(url: string): Promise<void> {
    try {
      await call(ThreadService.OpenExternal(url))
    } catch (e) {
      ui.pushToast('error', parseBindingError(e).message || msg.update.openFailed)
    }
  }

  function showUpdateBanner(info: UpdateInfo): void {
    const { version, url } = info
    if (!info.available || !version) {
      ui.clearBanner(UPDATE_BANNER_KEY)
      return
    }
    ui.setBanner({
      key: UPDATE_BANNER_KEY,
      kind: 'update',
      message: msg.update.banner(version),
      action: url
        ? { label: msg.update.bannerAction, run: () => void openExternal(url) }
        : undefined,
    })
  }

  function applyUpdate(info: UpdateInfo): void {
    update.value = info
    showUpdateBanner(info)
  }

  /** AI 設定とアプリ情報を読み込む。失敗時はエラートーストを出して投げ直さない。 */
  async function load(): Promise<void> {
    try {
      const [cfg, info] = await Promise.all([
        call(AIService.GetAIConfig()),
        call(AppService.GetAppInfo()),
      ])
      aiConfig.value = normalizeAIConfig(cfg)
      appInfo.value = info
    } catch (e) {
      ui.pushToast('error', parseBindingError(e).message || msg.loadFailed)
    }
  }

  /** 冪等。`update:available` を購読して初期ロードする。 */
  async function start(): Promise<void> {
    if (started) return
    started = true
    stopUpdateEvents = onEvent<UpdateAvailablePayload>(EVENTS.UPDATE_AVAILABLE, (p) => {
      applyUpdate({ available: true, version: p.version, url: p.url })
    })
    await load()
  }

  /** テスト・HMR 用の購読解除。 */
  function stop(): void {
    stopUpdateEvents?.()
    stopUpdateEvents = undefined
    started = false
  }

  async function setAIConfig(provider: string, model: string, baseUrl: string): Promise<void> {
    aiConfig.value = normalizeAIConfig(await call(AIService.SetAIConfig(provider, model, baseUrl)))
  }

  async function setAIApiKey(provider: string, apiKey: string): Promise<void> {
    aiConfig.value = normalizeAIConfig(await call(AIService.SetAIApiKey(provider, apiKey)))
  }

  async function clearAIApiKey(provider: string): Promise<void> {
    aiConfig.value = normalizeAIConfig(await call(AIService.ClearAIApiKey(provider)))
  }

  async function recordAIConsent(provider: string): Promise<void> {
    aiConfig.value = normalizeAIConfig(await call(AIService.RecordAIConsent(provider)))
  }

  async function setUpdateCheckEnabled(enabled: boolean): Promise<void> {
    await call(AppService.SetUpdateCheckEnabled(enabled))
    if (appInfo.value) appInfo.value = { ...appInfo.value, updateCheckEnabled: enabled }
    if (!enabled) {
      update.value = undefined
      ui.clearBanner(UPDATE_BANNER_KEY)
    }
  }

  /** 更新を今すぐ確認する。結果を state とバナーに反映して返す。 */
  async function checkUpdate(): Promise<UpdateInfo> {
    const info = await call(AppService.CheckForUpdate())
    applyUpdate(info)
    return info
  }

  return {
    aiConfig,
    appInfo,
    update,
    load,
    start,
    stop,
    openExternal,
    setAIConfig,
    setAIApiKey,
    clearAIApiKey,
    recordAIConsent,
    setUpdateCheckEnabled,
    checkUpdate,
  }
})
