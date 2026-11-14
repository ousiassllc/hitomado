import { beforeEach, describe, expect, test } from 'bun:test'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { useInboxZeroStore } from '../../../../stores/inboxZero'
import InboxZeroReasons from './InboxZeroReasons.vue'

beforeEach(() => {
  document.body.innerHTML = ''
})

describe('設定: Inbox Zero で不要とみなす理由（#314・#377）', () => {
  function mountSection() {
    const pinia = createPinia()
    setActivePinia(pinia)
    const zero = useInboxZeroStore()
    zero.loadReasons = async () => undefined
    const saved: unknown[] = []
    zero.setReasons = async (r) => {
      saved.push({ ...r })
      zero.reasons = { ...r }
    }
    const w = mount(InboxZeroReasons, { global: { plugins: [pinia] }, attachTo: document.body })
    return { w, zero, saved }
  }

  const sw = (w: ReturnType<typeof mountSection>['w'], r: string) =>
    w.get(`[data-testid="inbox-zero-reason-${r}"]`)

  test('5 つの理由（カレンダーを含む）がスイッチで並び、既定はすべてオン', () => {
    const { w } = mountSection()
    const group = w.get('[data-testid="inbox-zero-reasons"]')
    for (const [r, label] of [
      ['ad', '広告・宣伝'],
      ['notification', '通知だけのメール'],
      ['newsletter', '未読のニュースレター'],
      ['replied', '返信済みのメール'],
      ['calendar', 'カレンダーの招待・通知'],
    ] as const) {
      expect(group.text()).toContain(label)
      expect(sw(w, r).attributes('aria-checked')).toBe('true')
    }
    w.unmount()
  })

  test('切り替えると、その理由だけを変えて保存する（ほかの理由は保ったまま）', async () => {
    const { w, saved, zero } = mountSection()
    await sw(w, 'calendar').trigger('click')
    await flushPromises()
    expect(saved).toEqual([
      { ad: true, notification: true, newsletter: true, replied: true, calendar: false },
    ])
    expect(sw(w, 'calendar').attributes('aria-checked')).toBe('false')

    await sw(w, 'replied').trigger('click')
    await flushPromises()
    expect(zero.reasons).toEqual({
      ad: true,
      notification: true,
      newsletter: true,
      replied: false,
      calendar: false,
    })
    w.unmount()
  })

  test('保存に失敗したら、画面の値は変えずにエラーを出す', async () => {
    const { w, zero } = mountSection()
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
