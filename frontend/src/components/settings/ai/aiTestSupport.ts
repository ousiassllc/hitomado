import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import type {
  AIConfig,
  AIModelList,
} from '../../../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import { useSettingsStore } from '../../../stores/settings'
import AISettings from './AISettings.vue'

export { flushPromises }

export const base: AIConfig = {
  provider: 'openai',
  model: 'gpt-old',
  hasApiKey: true,
  consentedProviders: ['openai'],
  chatgptSignedIn: false,
  chatgptAccounts: [],
}

export function setup(cfg: Partial<AIConfig> = {}, models?: Record<string, AIModelList>) {
  const pinia = createPinia()
  setActivePinia(pinia)
  const store = useSettingsStore()
  store.aiConfig = { ...base, ...cfg }
  if (models) store.aiModels = models
  const calls: string[] = []
  store.loadAIModels = async (p: string) => {
    calls.push(`models:${p}`)
    return { provider: p, models: [] }
  }
  return { pinia, store, calls }
}

export const mountAI = (pinia: ReturnType<typeof createPinia>) =>
  mount(AISettings, { global: { plugins: [pinia] } })

export function button(w: VueWrapper, text: string) {
  const b = w.findAll('button').find((x) => x.text().includes(text))
  if (!b) throw new Error(`button not found: ${text}`)
  return b
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
