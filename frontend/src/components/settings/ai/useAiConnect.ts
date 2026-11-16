// 設定画面の「AI のつなぎ先」の、1 つのプロバイダをつなぐ流れ（#376。旧 useAiSetupFlow・useAiDraftState の後継。モデルの選択・取得は含まない）。
// つなぐ → (未同意なら) 同意の画面 → 同意のあとログイン（ChatGPT）または API キーの入力（ほか）へ進む。
import { computed, type Ref } from 'vue'
import { hasProviderCredential } from '../../../lib/aiConfig'
import { parseBindingError } from '../../../lib/api'
import { settings as msg } from '../../../lib/messages'
import { useSettingsStore } from '../../../stores/settings'
import { useUiStore } from '../../../stores/ui'
import type AIApiKey from './AIApiKey.vue'
import type AIChatGPTLogin from './AIChatGPTLogin.vue'
import { aiDestination, providerLabel } from './aiDestination'

/** 同意の画面（ダイアログ）が閉じ終わってから fn を呼ぶ（閉じるときに、ダイアログが開く前のフォーカスへ戻すため。最大 1 秒待つ）。 */
function afterDialogClosed(fn: () => void): void {
  const started = Date.now()
  const tick = (): void => {
    if (document.querySelector('[role="dialog"]') === null || Date.now() - started > 1000) fn()
    else setTimeout(tick, 30)
  }
  setTimeout(tick, 0)
}

/** 処理中の種類（同時に 1 つだけ。undefined は処理中でない）。 */
export type AiBusy = 'key' | 'clear' | 'consent' | 'baseUrl'

export interface AiConnectDeps {
  provider: Ref<string>
  busy: Ref<AiBusy | undefined>
  errors: Ref<{ key?: string; consent?: string; baseUrl?: string }>
  consentOpen: Ref<boolean>
  keyForm: Ref<InstanceType<typeof AIApiKey> | undefined>
  chatgptLogin: Ref<InstanceType<typeof AIChatGPTLogin> | undefined>
}

export function useAiConnect(deps: AiConnectDeps) {
  const store = useSettingsStore()
  const ui = useUiStore()
  const { provider, busy, errors, consentOpen, keyForm, chatgptLogin } = deps

  const cfg = computed(() => store.aiConfig)
  const isChatGPT = computed(() => provider.value === 'chatgpt')
  const hasCredential = computed(() => hasProviderCredential(provider.value, cfg.value))
  /** 保存済みの Base URL（openai だけ）。同意はこの送信先に対して取る。 */
  const baseUrl = computed(() => (provider.value === 'openai' ? (cfg.value?.baseUrl ?? '') : ''))
  const consented = computed(() => (cfg.value?.consentedProviders ?? []).includes(provider.value))
  const destination = computed(() => aiDestination(provider.value, baseUrl.value))
  const consentText = computed(() => {
    const label = providerLabel(provider.value)
    return consented.value ? msg.ai.consentGiven(label) : msg.ai.consentNeeded(label)
  })

  /** 同意とログイン・キーの案内（同意済みなら、資格情報が無いときだけ入力へ進める）。login=false は、ChatGPT のログインを自動で始めない。 */
  function guideCredentials(login: boolean): void {
    if (hasCredential.value) return
    if (isChatGPT.value) {
      if (login) void chatgptLogin.value?.login()
      return
    }
    keyForm.value?.focus()
  }

  /** 同意の画面を閉じたあとの案内。キー入力欄のフォーカスは、ダイアログがフォーカスを戻し終えてから当てる。 */
  function guideAfterConsent(): void {
    if (isChatGPT.value) guideCredentials(true)
    else afterDialogClosed(() => guideCredentials(true))
  }

  /** つなぐ流れの入口。未同意なら同意の画面を出し、同意済みならログイン（ChatGPT）・キーの入力へ進む。 */
  function begin(login = true): void {
    if (!consented.value) {
      consentOpen.value = true
      return
    }
    guideCredentials(login)
  }

  /** 同意の画面の「同意する」。同意を保存し、続けてログイン・キーの入力へ進む。 */
  async function agree(): Promise<void> {
    if (busy.value) return
    errors.value = {}
    busy.value = 'consent'
    try {
      await store.recordAIConsent(provider.value, baseUrl.value)
    } catch (e) {
      errors.value = { consent: parseBindingError(e).message }
      consentOpen.value = false
      return
    } finally {
      busy.value = undefined
    }
    consentOpen.value = false
    guideAfterConsent()
  }

  function declineConsent(): void {
    consentOpen.value = false
  }

  async function saveKey(apiKey: string): Promise<void> {
    if (busy.value) return
    errors.value = {}
    busy.value = 'key'
    try {
      await store.setAIApiKey(provider.value, apiKey)
      keyForm.value?.reset()
      ui.pushToast('success', msg.ai.apiKeySaved)
    } catch (e) {
      errors.value = { key: parseBindingError(e).message }
      return
    } finally {
      busy.value = undefined
    }
    if (!consented.value) consentOpen.value = true // キーを登録したら、未同意の送信先への同意を求める
  }

  async function clearKey(): Promise<void> {
    if (busy.value) return
    errors.value = {}
    busy.value = 'clear'
    try {
      await store.clearAIApiKey(provider.value)
      keyForm.value?.reset()
      ui.pushToast('success', msg.ai.apiKeyCleared)
    } catch (e) {
      errors.value = { key: parseBindingError(e).message }
    } finally {
      busy.value = undefined
    }
  }

  return {
    cfg,
    isChatGPT,
    hasCredential,
    baseUrl,
    consented,
    destination,
    consentText,
    begin,
    agree,
    declineConsent,
    saveKey,
    clearKey,
  }
}
