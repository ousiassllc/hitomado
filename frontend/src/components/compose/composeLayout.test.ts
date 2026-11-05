import { beforeEach, describe, expect, test } from 'bun:test'
import { compileStyle, parse } from '@vue/compiler-sfc'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import type { ComposeSession } from '../../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import { useComposeStore } from '../../stores/compose'
import ComposePane from './ComposePane.vue'
import RichTextEditor from './RichTextEditor.vue'

// 作成画面の本文エディタの大きさ（F-11「本文の入力欄の大きさ」。#112）。
// テストの SFC ローダーは <style> を取り込まないので、本番（vite）と同じく scoped を適用した CSS を入れて getComputedStyle で確かめる。

beforeEach(() => {
  document.body.innerHTML = ''
  document.head.innerHTML = ''
  setActivePinia(createPinia())
})

async function applyStyles(file: string, scopeId: string): Promise<void> {
  const path = new URL(file, import.meta.url).pathname
  const { descriptor } = parse(await Bun.file(path).text(), { filename: path })
  for (const s of descriptor.styles) {
    const el = document.createElement('style')
    el.textContent = compileStyle({
      source: s.content,
      filename: path,
      id: scopeId,
      scoped: s.scoped === true,
    }).code
    document.head.append(el)
  }
}

function replySession(): ComposeSession {
  return {
    sessionId: 's1',
    mode: 'reply',
    accountId: 'a1',
    from: { email: 'me@example.com' },
    senderLocked: true,
    content: {
      to: [{ email: 'to@example.com' }],
      cc: null,
      bcc: null,
      subject: 's',
      bodyHtml: '<p>b</p>',
    },
    attachments: [],
  }
}

/** scoped CSS を効かせるため、コンポーネントに scope id を付けて mount する（終わったら外す）。 */
async function mountWithStyles(props: { embedded?: boolean; expanded?: boolean } = {}) {
  await applyStyles('./ComposePane.vue', 'data-v-pane')
  await applyStyles('./RichTextEditor.vue', 'data-v-rte')
  const pane = ComposePane as { __scopeId?: string }
  const rte = RichTextEditor as { __scopeId?: string }
  pane.__scopeId = 'data-v-pane'
  rte.__scopeId = 'data-v-rte'
  const store = useComposeStore()
  store.session = replySession()
  store.content = {
    to: [{ email: 'to@example.com' }],
    cc: [],
    bcc: [],
    subject: 's',
    bodyHtml: '<p>b</p>',
  }
  const w = mount(ComposePane, { props, attachTo: document.body })
  await flushPromises()
  return {
    w,
    done: () => {
      w.unmount()
      delete pane.__scopeId
      delete rte.__scopeId
    },
  }
}

describe('本文エディタの最低高さ', () => {
  test('エディタは 240px 未満に縮まない。余った高さは広がる。RichTextEditor 側の指定で上書きされない', async () => {
    const { w, done } = await mountWithStyles()
    const editor = getComputedStyle(w.get('.compose-editor').element)
    expect(editor.minHeight).toBe('240px')
    expect(editor.flexShrink).toBe('0')
    expect(editor.flexBasis).toBe('240px')
    expect(editor.flexGrow).toBe('1')
    done()
  })

  test('足りない分は作成画面全体が縦にスクロールする。ほかの部品はエディタを押し縮めない', async () => {
    const { w, done } = await mountWithStyles()
    expect(getComputedStyle(w.get('.compose-pane').element).overflowY).toBe('auto')
    for (const sel of ['.compose-head', '.compose-fields', '.compose-foot', '.ai-draft']) {
      expect(getComputedStyle(w.get(sel).element).flexShrink).toBe('0')
    }
    done()
  })

  test('埋め込み（スレッド内）は右ペインの 60% までで、広げると上限を外して全体を使う', async () => {
    const normal = await mountWithStyles({ embedded: true })
    const embedded = getComputedStyle(normal.w.get('.compose-pane').element)
    expect(embedded.maxHeight).toBe('60%')
    expect(embedded.flexGrow).toBe('0')
    expect(getComputedStyle(normal.w.get('.compose-editor').element).minHeight).toBe('240px') // 上限より最低高さが優先
    normal.done()

    const wide = await mountWithStyles({ embedded: true, expanded: true })
    const expanded = getComputedStyle(wide.w.get('.compose-pane').element)
    expect(expanded.maxHeight).toBe('none')
    expect(expanded.flexGrow).toBe('1')
    wide.done()
  })

  test('埋め込みでないとき（新規作成・1 画面ずつ）は 60% の上限を付けず、残りの高さを使う', async () => {
    const { w, done } = await mountWithStyles()
    const pane = getComputedStyle(w.get('.compose-pane').element)
    expect(pane.maxHeight).not.toBe('60%')
    expect(pane.flexGrow).toBe('1')
    done()
  })
})

describe('「広げる／元に戻す」ボタン', () => {
  test('埋め込みのときだけ出す。新規作成・1 画面ずつの作成画面には出さない', async () => {
    const store = useComposeStore()
    store.session = replySession()
    const plain = mount(ComposePane, { attachTo: document.body })
    await flushPromises()
    expect(plain.find('[data-testid="compose-expand"]').exists()).toBe(false)
    plain.unmount()

    const emb = mount(ComposePane, { props: { embedded: true }, attachTo: document.body })
    await flushPromises()
    const btn = emb.get('[data-testid="compose-expand"]')
    expect(btn.text()).toBe('広げる')
    expect(btn.attributes('aria-pressed')).toBe('false')
    await btn.trigger('click')
    expect(emb.emitted('toggle-expand')).toHaveLength(1)
    await emb.setProps({ expanded: true })
    expect(emb.get('[data-testid="compose-expand"]').text()).toBe('元に戻す')
    expect(emb.get('[data-testid="compose-expand"]').attributes('aria-pressed')).toBe('true')
    emb.unmount()
  })
})
