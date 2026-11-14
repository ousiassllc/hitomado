import { afterAll, beforeEach, describe, expect, test } from 'bun:test'
import { flushPromises, mount } from '@vue/test-utils'
import {
  appError,
  callsOf,
  failures,
  installMocks,
  resetKit,
  responses,
  restoreMocks,
} from '../../../test/inboxKit'

installMocks()
afterAll(restoreMocks)

const { useAgentStore } = await import('../../stores/agent')
const { usePlatformStore } = await import('../../stores/platform')
const { useSettingsStore } = await import('../../stores/settings')
const { useUiStore } = await import('../../stores/ui')
const { default: AgentSettings } = await import('./AgentSettings.vue')

beforeEach(() => {
  resetKit()
  document.body.innerHTML = ''
})

const state = (over: Record<string, unknown> = {}) => ({
  enabled: false,
  mascotEnabled: false,
  aiReady: true,
  ...over,
})

async function mountPage(over: Record<string, unknown> = {}, supportsMascot = false) {
  responses.GetAgentSettings = state(over)
  usePlatformStore().capabilities = { ...usePlatformStore().capabilities, supportsMascot }
  useSettingsStore().aiConfig = { provider: 'anthropic', model: 'm', configured: true } as never
  const w = mount(AgentSettings, { attachTo: document.body })
  await flushPromises()
  return { w, ui: useUiStore(), store: useAgentStore() }
}

const sw = (w: Awaited<ReturnType<typeof mountPage>>['w'], testid: string) =>
  w.get(`[data-testid=${testid}] [role=switch]`)
const body = (testid: string) => document.body.querySelector(`[data-testid=${testid}]`)

describe('設定のエージェントのページ', () => {
  test('既定はオフ。オンにする操作で、送信先を示す同意を出し、同意で setEnabled(true, true) を呼ぶ', async () => {
    const { w, store } = await mountPage()
    expect(sw(w, 'agent-enabled').attributes('aria-checked')).toBe('false')
    await sw(w, 'agent-enabled').trigger('click')
    await flushPromises()
    expect(callsOf('SetAgentEnabled')).toHaveLength(0) // 同意の前は呼ばない
    const text = body('agent-consent-body')?.textContent ?? ''
    expect(text).toContain('Anthropic')
    expect(text).toContain('https://api.anthropic.com')
    expect(text).toContain('送信は必ず自分で押します')

    responses.SetAgentEnabled = state({ enabled: true })
    responses.GetAgentSettings = state({ enabled: true })
    ;(body('agent-consent-agree') as HTMLElement).click()
    await flushPromises()
    expect(callsOf('SetAgentEnabled')[0]?.args).toEqual([true, true])
    expect(store.enabled).toBe(true)
    expect(body('agent-consent-body')).toBeNull()
    w.unmount()
  })

  test('同意をキャンセルしたらオンにならない', async () => {
    const { w, store } = await mountPage()
    await sw(w, 'agent-enabled').trigger('click')
    await flushPromises()
    ;(body('agent-consent-cancel') as HTMLElement).click()
    await flushPromises()
    expect(callsOf('SetAgentEnabled')).toHaveLength(0)
    expect(store.enabled).toBe(false)
    expect(sw(w, 'agent-enabled').attributes('aria-checked')).toBe('false')
    w.unmount()
  })

  test('AI が使えない（aiReady が偽）ときは同意の代わりに AI の設定へ案内する', async () => {
    const { w, ui } = await mountPage({ aiReady: false })
    await sw(w, 'agent-enabled').trigger('click')
    await flushPromises()
    expect(body('agent-consent-body')).toBeNull()
    expect(document.body.textContent).toContain('先に AI の設定が必要です')
    ;(body('agent-need-ai-open') as HTMLElement).click()
    await flushPromises()
    expect(ui.settingsTab).toBe('ai')
    expect(ui.dialogs.settings).toBe(true)
    expect(callsOf('SetAgentEnabled')).toHaveLength(0)
    w.unmount()
  })

  test.each(['AI_NOT_CONFIGURED', 'AI_CONSENT_REQUIRED'])(
    '同意後に Go が %s を返したら、AI の設定への案内に切り替える',
    async (code) => {
      const { w, store } = await mountPage()
      await sw(w, 'agent-enabled').trigger('click')
      await flushPromises()
      failures.SetAgentEnabled = appError(code)
      ;(body('agent-consent-agree') as HTMLElement).click()
      await flushPromises()
      expect(body('agent-consent-body')).toBeNull()
      expect(body('agent-need-ai-open')).not.toBeNull()
      expect(store.enabled).toBe(false)
      w.unmount()
    },
  )

  test('その他の失敗はアラートで知らせる', async () => {
    const { w, ui } = await mountPage()
    await sw(w, 'agent-enabled').trigger('click')
    await flushPromises()
    failures.SetAgentEnabled = appError('INTERNAL')
    ;(body('agent-consent-agree') as HTMLElement).click()
    await flushPromises()
    expect(ui.alerts.map((a) => a.message)).toEqual(['err INTERNAL'])
    w.unmount()
  })

  test('オフにするのは同意なしで、すぐ setEnabled(false, false) を呼ぶ', async () => {
    const { w, store } = await mountPage({ enabled: true })
    expect(sw(w, 'agent-enabled').attributes('aria-checked')).toBe('true')
    responses.SetAgentEnabled = state()
    responses.GetAgentSettings = state()
    await sw(w, 'agent-enabled').trigger('click')
    await flushPromises()
    expect(callsOf('SetAgentEnabled')[0]?.args).toEqual([false, false])
    expect(store.enabled).toBe(false)
    w.unmount()
  })

  test('オンの間に AI が使えない状態なら注意を出し、AI の設定へ進める', async () => {
    const { w, ui } = await mountPage({ enabled: true, aiReady: false })
    expect(w.get('[data-testid=agent-ai-not-ready]').text()).toContain('エージェントは動きません')
    await w.get('[data-testid=agent-ai-settings]').trigger('click')
    expect(ui.settingsTab).toBe('ai')
    w.unmount()
  })

  test('AI が使えるオンの間は注意を出さない', async () => {
    const { w } = await mountPage({ enabled: true })
    expect(w.find('[data-testid=agent-ai-not-ready]').exists()).toBe(false)
    w.unmount()
  })
})

describe('設定のマスコット', () => {
  test('マスコットを出せない OS（Android など）では出さない', async () => {
    const { w } = await mountPage({ enabled: true }, false)
    expect(w.find('[data-testid=agent-mascot]').exists()).toBe(false)
    w.unmount()
  })

  test('エージェントがオフなら無効にして説明を出し、オンなら切り替えられる', async () => {
    const off = await mountPage({}, true)
    expect(sw(off.w, 'agent-mascot').attributes('disabled')).toBeDefined()
    expect(off.w.get('[data-testid=agent-mascot-needs-agent]').text()).toContain('先に')
    off.w.unmount()

    const { w, store } = await mountPage({ enabled: true }, true)
    expect(sw(w, 'agent-mascot').attributes('disabled')).toBeUndefined()
    expect(w.find('[data-testid=agent-mascot-needs-agent]').exists()).toBe(false)
    responses.SetAgentMascot = state({ enabled: true, mascotEnabled: true })
    await sw(w, 'agent-mascot').trigger('click')
    await flushPromises()
    expect(callsOf('SetAgentMascot')[0]?.args).toEqual([true])
    expect(store.settings.mascotEnabled).toBe(true)
    w.unmount()
  })
})
