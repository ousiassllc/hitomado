import { defineStore } from 'pinia'
import { ref } from 'vue'
import {
  AIService,
  AppService,
  LogService,
  MCPService,
  ThreadService,
} from '../../bindings/github.com/ousiassllc/hitomado/internal/app'
import type {
  AIConfig,
  AIModelList,
  AppInfo,
  ExportLogsResult,
  MCPConfig,
  MCPStatus,
  UpdateInfo,
} from '../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import { call, list, parseBindingError } from '../lib/api'
import { EVENTS, onEvent, type UpdateAvailablePayload } from '../lib/events'
import { settings as msg } from '../lib/messages'
import { useUiStore } from './ui'

const UPDATE_BANNER_KEY = 'update'

/** モデル一覧の保存キー。OpenAI 互換は Base URL ごとに別の一覧になる（Base URL が空の既定の送信先は、プロバイダ名だけ）。 */
export function aiModelsKey(provider: string, baseUrl = ''): string {
  const url = baseUrl.trim()
  return provider === 'openai' && url !== '' ? `${provider}|${url}` : provider
}

/** Go は consentedProviders・apiKeyProviders・chatgptAccounts を nil にし得るので、画面側は常に配列として扱う。 */
function normalizeAIConfig(c: AIConfig): AIConfig {
  return {
    ...c,
    consentedProviders: list(c.consentedProviders),
    apiKeyProviders: list(c.apiKeyProviders),
    chatgptAccounts: list(c.chatgptAccounts),
  }
}

export const useSettingsStore = defineStore('settings', () => {
  const ui = useUiStore()
  const aiConfig = ref<AIConfig>()
  /** 送信先（プロバイダ・OpenAI 互換は Base URL。`aiModelsKey`）ごとの、最後に取得したモデル一覧。 */
  const aiModels = ref<Record<string, AIModelList>>({})
  const appInfo = ref<AppInfo>()
  const update = ref<UpdateInfo>()
  const mcp = ref<MCPStatus>()
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

  /** MCP の設定と待ち受けの状態を読み込む。失敗時はエラートーストを出して投げ直さない。 */
  async function loadMcp(): Promise<void> {
    try {
      mcp.value = await call(MCPService.GetMCPStatus())
    } catch (e) {
      ui.pushToast('error', parseBindingError(e).message || msg.loadFailed)
    }
  }

  async function setMcpConfig(cfg: MCPConfig): Promise<void> {
    mcp.value = await call(MCPService.SetMCPConfig(cfg))
  }

  /** トークンを（再）発行して値を返す。値は呼び出し側が 1 回だけ表示し、ストアには残さない。 */
  async function issueMcpToken(): Promise<string> {
    const res = await call(MCPService.IssueMCPToken())
    mcp.value = res.status
    return res.token
  }

  async function revokeMcpToken(): Promise<void> {
    mcp.value = await call(MCPService.RevokeMCPToken())
  }

  async function openLogFolder(): Promise<void> {
    await call(LogService.OpenLogFolder())
  }

  async function exportLogs(): Promise<ExportLogsResult> {
    return await call(LogService.ExportLogs())
  }

  /** 冪等。`update:available` を購読して初期ロードする。 */
  async function start(): Promise<void> {
    if (started) return
    started = true
    stopUpdateEvents = onEvent<UpdateAvailablePayload>(EVENTS.UPDATE_AVAILABLE, (p) => {
      applyUpdate({ available: true, version: p.version, url: p.url })
    })
    await load()
    // Go の起動時確認の `update:available` は購読前に発行され得る（再送なし）ため、ここで一度確認する。
    if (appInfo.value?.updateCheckEnabled) await checkUpdateQuietly()
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

  /** 送信への同意を保存する。baseUrl は OpenAI 互換のみ有効で、保存前に選んだ送信先にも同意できる。 */
  async function recordAIConsent(provider: string, baseUrl = ''): Promise<void> {
    aiConfig.value = normalizeAIConfig(await call(AIService.RecordAIConsent(provider, baseUrl)))
  }

  /**
   * 選んだ送信先（provider と、OpenAI 互換の baseUrl）のモデル一覧を取得する。保存前の設定でも取得できる。
   * 取得できないときも例外にせず、reason つきの空の一覧が入る（手入力へ切り替える）。同意前の送信先は Go が取得を送らず
   * reason=consent_required を返す。
   */
  async function loadAIModels(provider: string, baseUrl = ''): Promise<AIModelList> {
    const r = await call(AIService.ListAIModels(provider, baseUrl))
    const res = { ...r, models: list(r.models) }
    aiModels.value = { ...aiModels.value, [aiModelsKey(provider, baseUrl)]: res }
    return res
  }

  /** ChatGPT にログインする。accountId が空なら別のアカウントを追加し、指定したときはその登録で再ログインする。 */
  async function signInChatGPT(accountId: string): Promise<void> {
    aiConfig.value = normalizeAIConfig(await call(AIService.SignInChatGPT(accountId)))
  }

  async function cancelSignInChatGPT(): Promise<void> {
    await call(AIService.CancelSignInChatGPT())
  }

  /** 下書きに使う ChatGPT アカウントを切り替える（ログイン中のアカウントだけ）。 */
  async function selectChatGPTAccount(accountId: string): Promise<void> {
    aiConfig.value = normalizeAIConfig(await call(AIService.SelectChatGPTAccount(accountId)))
  }

  /** 選んだ ChatGPT アカウントだけログアウトする。OpenAI 側の失効を確認できたかを返す。 */
  async function signOutChatGPT(accountId: string): Promise<boolean> {
    const r = await call(AIService.SignOutChatGPT(accountId))
    aiConfig.value = normalizeAIConfig(r.config)
    return r.revocationConfirmed
  }

  /** 返信の下書きを生成する（cancelGenerate で中止できる。requestId は呼び出しごとに一意）。 */
  async function generateReplyDraft(
    sessionId: string,
    requestId: string,
  ): Promise<{ bodyHtml: string }> {
    return await call(AIService.GenerateReplyDraft(sessionId, requestId))
  }

  async function cancelGenerate(requestId: string): Promise<void> {
    await call(AIService.CancelGenerate(requestId))
  }

  /** 「You're using your ChatGPT plan」の確認を済みにする（そのアカウントでは以後出さない）。 */
  async function ackChatGPTPlanNotice(accountId: string): Promise<void> {
    aiConfig.value = normalizeAIConfig(await call(AIService.AckChatGPTPlanNotice(accountId)))
  }

  /** ログアウト済みの ChatGPT アカウントの登録を忘れる。 */
  async function removeChatGPTAccount(accountId: string): Promise<void> {
    aiConfig.value = normalizeAIConfig(await call(AIService.RemoveChatGPTAccount(accountId)))
  }

  async function setUpdateCheckEnabled(enabled: boolean): Promise<void> {
    await call(AppService.SetUpdateCheckEnabled(enabled))
    if (appInfo.value) appInfo.value = { ...appInfo.value, updateCheckEnabled: enabled }
    if (!enabled) {
      update.value = undefined
      ui.clearBanner(UPDATE_BANNER_KEY)
      return
    }
    await checkUpdateQuietly()
  }

  /**
   * 背景での更新確認（起動時・ON 復帰時）。更新があるときだけ反映し、失敗（オフライン等）は通知しない。
   * 手動確認は `checkUpdate` が結果とエラーを呼び出し側へ返す。
   */
  async function checkUpdateQuietly(): Promise<void> {
    try {
      const info = await call(AppService.CheckForUpdate())
      if (info.available) applyUpdate(info)
    } catch {
      // 設定の変更・起動自体は成功している。次回の確認で再試行される。
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
    aiModels,
    appInfo,
    update,
    mcp,
    load,
    loadMcp,
    setMcpConfig,
    issueMcpToken,
    revokeMcpToken,
    openLogFolder,
    exportLogs,
    start,
    stop,
    openExternal,
    setAIConfig,
    setAIApiKey,
    clearAIApiKey,
    recordAIConsent,
    loadAIModels,
    signInChatGPT,
    cancelSignInChatGPT,
    selectChatGPTAccount,
    signOutChatGPT,
    removeChatGPTAccount,
    ackChatGPTPlanNotice,
    generateReplyDraft,
    cancelGenerate,
    setUpdateCheckEnabled,
    checkUpdate,
  }
})
