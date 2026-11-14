import { beforeEach, describe, expect, test } from 'bun:test'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { useInboxZeroStore } from '../../../../stores/inboxZero'
import { useSettingsStore } from '../../../../stores/settings'
import { base } from '../aiTestSupport'
import AITypeSafePage from './AITypeSafePage.vue'

beforeEach(() => {
  document.body.innerHTML = ''
})

describe('設定: Inbox Zero で不要とみなす理由（#314）', () => {
  function mountPage() {
    const pinia = createPinia()
    setActivePinia(pinia)
    const store = useSettingsStore()
    store.aiConfig = { ...base, apiKeyProviders: ['typesafe'] }
    const zero = useInboxZeroStore()
    zero.loadReasons = async () => undefined
    const saved: unknown[] = []
    zero.setReasons = async (r) => {
      saved.push({ ...r })
      zero.reasons = { ...r }
    }
    const w = mount(AITypeSafePage, { global: { plugins: [pinia] }, attachTo: document.body })
    return { w, zero, saved }
  }

  const sw = (w: ReturnType<typeof mountPage>['w'], r: string) =>
    w.get(`[data-testid="inbox-zero-reason-${r}"]`)

  test('4 つの理由がスイッチで並び、既定はすべてオン', () => {
    const { w } = mountPage()
    const group = w.get('[data-testid="inbox-zero-reasons"]')
    for (const [r, label] of [
      ['ad', '広告・宣伝'],
      ['notification', '通知だけのメール'],
      ['newsletter', '未読のニュースレター'],
      ['replied', '返信済みのメール'],
    ] as const) {
      expect(group.text()).toContain(label)
      expect(sw(w, r).attributes('aria-checked')).toBe('true')
    }
    w.unmount()
  })

  test('切り替えると、その理由だけを変えて保存する（ほかの理由は保ったまま）', async () => {
    const { w, saved, zero } = mountPage()
    await sw(w, 'newsletter').trigger('click')
    await flushPromises()
    expect(saved).toEqual([{ ad: true, notification: true, newsletter: false, replied: true }])
    expect(sw(w, 'newsletter').attributes('aria-checked')).toBe('false')

    await sw(w, 'replied').trigger('click')
    await flushPromises()
    expect(zero.reasons).toEqual({
      ad: true,
      notification: true,
      newsletter: false,
      replied: false,
    })
    w.unmount()
  })

  test('保存に失敗したら、画面の値は変えずにエラーを出す', async () => {
    const { w, zero } = mountPage()
    zero.setReasons = async () => {
      throw new Error('boom')
    }
    await sw(w, 'ad').trigger('click')
    await flushPromises()
    expect(w.get('[role="alert"]').text().length).toBeGreaterThan(0)
    expect(sw(w, 'ad').attributes('aria-checked')).toBe('true')
    w.unmount()
  })
})
