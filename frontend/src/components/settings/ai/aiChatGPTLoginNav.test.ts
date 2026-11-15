import { beforeEach, describe, expect, test } from 'bun:test'
import { flushPromises, mount } from '@vue/test-utils'
import type { Pinia } from 'pinia'
import type { ChatGPTAccount } from '../../../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import { useUiStore } from '../../../stores/ui'
import { acctA, acctB, base, chatgptCfg, setup } from './aiTestSupport'
import AIProviders from './providers/AIProviders.vue'

// ChatGPT でログインできたら、つなぎ先のページから「機能ごとに使う AI」のタブへ移る（#376）。
// ページが閉じても、OpenAI の「ChatGPT プランを使っています」の確認は出続ける（AIProviders にある）。
beforeEach(() => {
  document.body.innerHTML = ''
  localStorage.clear()
})

const mountTab = (pinia: Pinia) =>
  mount(AIProviders, { global: { plugins: [pinia] }, attachTo: document.body })

/** ChatGPT のページを開いた状態（つなぎ先のタブ）で AIProviders を載せる。 */
function openChatGPTPage(accounts: ChatGPTAccount[]) {
  const s = setup({ ...chatgptCfg(accounts), consentedProviders: ['chatgpt'] })
  const ui = useUiStore()
  ui.openAiProvider('chatgpt')
  return { ...s, ui, w: mountTab(s.pinia) }
}

const planNotice = () => document.body.querySelector('[data-testid="ai-chatgpt-plan-notice"]')

describe('ChatGPT のログインに成功したあと', () => {
  test('最初のログイン: ページを閉じて「機能ごとに使う AI」へ移り、プランの確認はそのまま出る', async () => {
    const { w, store, ui } = openChatGPTPage([])
    store.signInChatGPT = async () => {
      store.aiConfig = { ...base, ...chatgptCfg([{ ...acctA, planNoticePending: true }]) }
    }
    await flushPromises()
    expect(ui.aiProviderPage).toBe('chatgpt')
    expect(planNotice()).toBeNull()

    await w.get('[data-testid="ai-chatgpt-login"]').trigger('click')
    await flushPromises()

    expect(ui.aiProviderPage).toBeUndefined()
    expect(ui.aiTab).toBe('use')
    expect(w.find('[data-testid="ai-chatgpt"]').exists()).toBe(false)
    expect(w.find('[data-testid="ai-use-tab"]').exists()).toBe(true)
    expect(planNotice()).not.toBeNull()
    w.unmount()
  })

  test('アカウントの追加・再ログインでも、同じように「機能ごとに使う AI」へ移る', async () => {
    for (const trigger of ['ai-chatgpt-add', 'ai-chatgpt-relogin']) {
      document.body.innerHTML = ''
      const { w, store, ui } = openChatGPTPage([acctA, { ...acctB, signedIn: false }])
      const ids: string[] = []
      store.signInChatGPT = async (id: string) => {
        ids.push(id)
      }
      await flushPromises()
      await w.get(`[data-testid="${trigger}"]`).trigger('click')
      await flushPromises()
      expect(ids).toEqual([trigger === 'ai-chatgpt-add' ? '' : 'oaiapp_BBBB'])
      expect(ui.aiProviderPage).toBeUndefined()
      expect(ui.aiTab).toBe('use')
      w.unmount()
    }
  })

  test('ログインの失敗・中止では移らない（ページに留まり、失敗は理由を出す）', async () => {
    const { w, store, ui } = openChatGPTPage([])
    store.signInChatGPT = async () => {
      throw { code: 'OAUTH_DENIED', message: '許可されませんでした。', retryable: false }
    }
    await flushPromises()
    await w.get('[data-testid="ai-chatgpt-login"]').trigger('click')
    await flushPromises()
    expect(ui.aiProviderPage).toBe('chatgpt')
    expect(ui.aiTab).toBe('connect')
    expect(w.get('[role="alert"]').text()).toContain('許可されませんでした')

    store.signInChatGPT = async () => {
      throw { code: 'OAUTH_CANCELLED', message: 'キャンセルしました。', retryable: false }
    }
    await w.get('[data-testid="ai-chatgpt-login"]').trigger('click')
    await flushPromises()
    expect(ui.aiProviderPage).toBe('chatgpt')
    expect(ui.aiTab).toBe('connect')
    expect(w.find('[data-testid="ai-chatgpt"]').exists()).toBe(true)
    w.unmount()
  })
})
