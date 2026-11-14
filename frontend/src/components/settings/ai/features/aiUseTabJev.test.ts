import { beforeEach, describe, expect, test } from 'bun:test'
import { flushPromises, mount } from '@vue/test-utils'
import { features } from '../../../../../test/aiConfigKit'
import { setup } from '../aiTestSupport'
import AIUseTab from './AIUseTab.vue'

beforeEach(() => {
  document.body.innerHTML = ''
  localStorage.clear()
})

const feat = (w: ReturnType<typeof mount>, id: string) => w.get(`[data-testid="ai-feature-${id}"]`)

describe('「機能ごとに使う AI」タブの Jev の固定表示（#376・#377）', () => {
  test('Jev のほかにもつないであるときは、モデル一覧の取得中に Jev を保存しない（取得が終わるまで「固定」にもしない）', async () => {
    const s = setup({
      apiKeyProviders: ['openai', 'typesafe'],
      consentedProviders: ['openai', 'typesafe'],
      features: features({ provider: 'openai', model: 'gpt-a' }),
      chatgptSignedIn: false,
      chatgptAccounts: [],
    })
    const finish: (() => void)[] = []
    const load = s.store.loadAIModels
    s.store.loadAIModels = (p: string, b = '') =>
      new Promise((resolve) => {
        finish.push(() => resolve(load(p, b)))
      })
    const saved: unknown[][] = []
    s.store.setAIFeature = async (...a: unknown[]) => {
      saved.push(a)
    }
    const w = mount(AIUseTab, { global: { plugins: [s.pinia] }, attachTo: document.body })
    await flushPromises()
    expect(finish).toHaveLength(1) // 取得の途中
    expect(feat(w, 'inboxZero').find('[data-testid="ai-feature-fixed"]').exists()).toBe(false)
    expect(saved).toEqual([])
    for (const f of finish) f()
    await flushPromises()
    expect(feat(w, 'inboxZero').find('[data-testid="ai-feature-fixed"]').exists()).toBe(false)
    expect(saved).toEqual([])
    w.unmount()
  })
})
