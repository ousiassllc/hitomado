import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import type {
  AIConfig,
  AIModelList,
} from '../../../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import { aiModelsKey } from '../../../lib/aiConfig'
import { useSettingsStore } from '../../../stores/settings'
import AISettings from './AISettings.vue'

export { flushPromises }

export const base: AIConfig = {
  provider: 'openai',
  model: 'gpt-old',
  configured: true,
  hasApiKey: true,
  apiKeyProviders: ['openai'],
  consentedProviders: ['openai'],
  chatgptSignedIn: false,
  chatgptAccounts: [],
}

/** 取得の結果を決める関数（provider・OpenAI 互換の baseUrl を受ける）。既定は、空の一覧（理由なし）。 */
export type ModelsFn = (provider: string, baseUrl: string) => AIModelList

export function setup(
  cfg: Partial<AIConfig> = {},
  models?: Record<string, AIModelList>,
  load?: ModelsFn,
) {
  const pinia = createPinia()
  setActivePinia(pinia)
  const store = useSettingsStore()
  store.aiConfig = { ...base, ...cfg }
  if (models) store.aiModels = models
  const calls: string[] = []
  /** 取得の呼び出し（provider と、そのとき渡した baseUrl）。 */
  const loads: { provider: string; baseUrl: string }[] = []
  store.loadAIModels = async (p: string, baseUrl = '') => {
    calls.push(`models:${p}`)
    loads.push({ provider: p, baseUrl })
    const r = load?.(p, baseUrl) ?? { provider: p, models: [] }
    store.aiModels = { ...store.aiModels, [aiModelsKey(p, baseUrl)]: r }
    return r
  }
  return { pinia, store, calls, loads }
}

/**
 * プロバイダの設定ページ（AISettings）を開く。一覧から開くのと同じく、既定は保存済み（未保存・未読み込みは ChatGPT）の
 * プロバイダのページ。別のプロバイダのページは `switchProvider` で開き直す（一覧から別のプロバイダを選んだのと同じ）。
 */
export const mountAI = (pinia: ReturnType<typeof createPinia>, provider?: string) =>
  mount(AISettings, {
    props: { provider: provider ?? useSettingsStore().aiConfig?.provider ?? 'chatgpt' },
    global: { plugins: [pinia] },
    attachTo: document.body,
  })

/** 一覧から別のプロバイダを選んだとき（ページのプロバイダが変わるとき）と同じ。 */
export const switchProvider = (w: VueWrapper, provider: string) => w.setProps({ provider })

/** 同意の画面（ダイアログは body 直下に出る）の要素。 */
export const consentEl = (testid: string) =>
  document.body.querySelector<HTMLElement>(`[data-testid="${testid}"]`)

export function button(w: VueWrapper, text: string) {
  const b = w.findAll('button').find((x) => x.text().includes(text))
  if (!b) throw new Error(`button not found: ${text}`)
  return b
}

/** 確認モーダル（ボディへテレポートされる）の中のボタンを文言で探す。 */
export function dialogButton(text: string): HTMLButtonElement | undefined {
  return [...document.body.querySelectorAll<HTMLButtonElement>('[role="dialog"] button')].find(
    (b) => b.textContent?.trim() === text,
  )
}

export const acctA = {
  id: 'oaiapp_AAAA',
  email: 'a@chatgpt.example',
  signedIn: true,
  active: true,
  planNoticePending: false,
}
export const acctB = {
  id: 'oaiapp_BBBB',
  email: 'b@chatgpt.example',
  signedIn: true,
  active: false,
  planNoticePending: false,
}

/** ChatGPT プロバイダで、保存済みのアカウントを持つ設定。使用中のアカウントに合わせて chatgptSignedIn・メールを入れる。 */
export function chatgptCfg(accounts: AIConfig['chatgptAccounts']): Partial<AIConfig> {
  const active = (accounts ?? []).find((a) => a.active)
  return {
    provider: 'chatgpt',
    hasApiKey: active?.signedIn === true,
    chatgptSignedIn: active?.signedIn === true,
    chatgptEmail: active?.email,
    chatgptAccounts: accounts,
  }
}
