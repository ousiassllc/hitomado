import { describe, expect, test } from 'bun:test'
import { parse } from '@vue/compiler-sfc'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import type { ThreadSummary } from '../../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import ThreadList from './ThreadList.vue'

setActivePinia(createPinia())

// #337: listbox（.inner）のフォーカスリングが、行（position した .slot）の背面に隠れて見えなかった。
// リングはスクロールしない枠（.frame）の ::after に、行より前面の重ねとして outline で描く。
// テストの SFC ローダーは <style> を取り込まないので、SFC の CSS を読んで確かめる。
async function css(): Promise<string> {
  const path = new URL('./ThreadList.vue', import.meta.url).pathname
  const { descriptor } = parse(await Bun.file(path).text(), { filename: path })
  return descriptor.styles.map((s) => s.content).join('\n')
}

function block(source: string, selector: string): string {
  const at = source.indexOf(`${selector} {`)
  expect(at).toBeGreaterThanOrEqual(0)
  return source.slice(at).match(/\{([^}]*)\}/)?.[1] ?? ''
}

describe('ThreadList の listbox フォーカスリング（#337）', () => {
  test('リングは、スクロールしない枠の ::after に、行より前面の pointer-events: none の重ねで描く', async () => {
    const style = await css()
    const overlay = block(style, '.frame::after')
    expect(overlay).toMatch(/position:\s*absolute;/)
    expect(overlay).toMatch(/inset:\s*0;/)
    expect(overlay).toMatch(/z-index:\s*1;/)
    expect(overlay).toMatch(/pointer-events:\s*none;/)
    // z-index が枠の外へ漏れないよう、枠が自分の積層文脈を作る
    expect(block(style, '.frame')).toMatch(/isolation:\s*isolate;/)
    expect(block(style, '.frame')).not.toMatch(/overflow/)
  })

  test('リングは --focus の 2px・内側（offset -2px）の outline（box-shadow は forced-colors で消える）', async () => {
    const style = await css()
    const ring = block(style, '.frame:has(.inner:focus-visible)::after')
    expect(ring).toMatch(/outline:\s*2px solid var\(--focus\);/)
    expect(ring).toMatch(/outline-offset:\s*-2px;/)
    expect(ring).not.toMatch(/box-shadow/)
  })

  test('.list（スクロール領域）自体には outline を描かない', async () => {
    const style = await css()
    expect(block(style, '.list')).not.toMatch(/outline:\s*\d/)
    expect(style).not.toMatch(/\.list:has\(/)
  })

  test('DOM は .frame > .list（スクロール）> listbox。リングの重ね先の .frame は .list の外にある', () => {
    const items: ThreadSummary[] = [
      {
        accountId: 'a1',
        threadId: 't1',
        subject: '見積のご確認',
        snippet: '',
        lastMessageAt: Date.now(),
        lastFrom: { name: '山田太郎', email: 'yamada@example.com' },
        messageCount: 1,
        isUnread: false,
        hasAttachment: false,
        inInbox: true,
        labels: [],
      },
    ]
    const w = mount(ThreadList, {
      props: {
        items,
        checkedKeys: [],
        loading: false,
        hasMore: false,
        loadingMore: false,
        emptyText: '',
      },
    })
    expect(w.find('.frame > .list > [role="listbox"]').exists()).toBe(true)
  })
})
