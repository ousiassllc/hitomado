import { afterAll, afterEach, beforeEach, describe, expect, mock, test, vi } from 'bun:test'
import { flushPromises, mount } from '@vue/test-utils'
import * as realCompose from '../../../../bindings/github.com/ousiassllc/hitomado/internal/app/composeservice'
import { SUGGEST_DELAY_MS } from '../../../composables/useContactSuggest'

// 宛先候補を ↑↓ で選んだとき、一覧（max-height でスクロールする）の中で選んだ候補が見える位置へスクロールする（ガイド B6）。

const B = '../../../../bindings/github.com/ousiassllc/hitomado/internal/app'
const candidates = Array.from({ length: 8 }, (_, i) => ({ email: `a${i}@example.com` }))
mock.module(`${B}/composeservice`, () => ({
  SuggestContacts: async () => candidates,
}))
afterAll(() => mock.module(`${B}/composeservice`, () => ({ ...realCompose })))

// mock.module の後に読み込む必要があるため静的 import にできない（他のテストと同じ）
const { default: RecipientField } = await import('./RecipientField.vue')

const scrolled: string[] = []
const original = HTMLElement.prototype.scrollIntoView
beforeEach(() => {
  vi.useFakeTimers() // 候補の取得は 150ms のデバウンス後
  scrolled.length = 0
  HTMLElement.prototype.scrollIntoView = function (
    this: HTMLElement,
    arg?: boolean | ScrollIntoViewOptions,
  ) {
    expect(arg).toEqual({ block: 'nearest' })
    scrolled.push(this.textContent ?? '')
  }
})
afterEach(() => {
  vi.useRealTimers()
  HTMLElement.prototype.scrollIntoView = original
  document.body.innerHTML = ''
})

describe('RecipientField 候補のキーボード操作', () => {
  test('↑↓ で選んだ候補をスクロールして見せる', async () => {
    const w = mount(RecipientField, {
      props: { modelValue: [], accountId: 'acc-1', label: '宛先' },
      attachTo: document.body,
    })
    const input = w.get('input')
    await input.setValue('a')
    vi.advanceTimersByTime(SUGGEST_DELAY_MS)
    await flushPromises()
    expect(w.findAll('[role="option"]')).toHaveLength(8)

    await input.trigger('keydown', { key: 'ArrowDown' })
    await flushPromises()
    expect(scrolled).toEqual(['a0@example.com'])

    await input.trigger('keydown', { key: 'ArrowUp' }) // 先頭から末尾へ回る
    await flushPromises()
    expect(scrolled).toEqual(['a0@example.com', 'a7@example.com'])
    w.unmount()
  })
})
