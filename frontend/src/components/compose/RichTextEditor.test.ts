import { describe, expect, test } from 'bun:test'
import type { Editor } from '@tiptap/vue-3'
import { flushPromises, mount } from '@vue/test-utils'
import { buildSignatureBlock, findSignature } from '../../lib/signature'
import RichTextEditor from './RichTextEditor.vue'

const QUOTE = '<blockquote data-hitomado-quote><p>引用</p></blockquote>'

async function mountEditor(html: string) {
  const w = mount(RichTextEditor, { props: { html }, attachTo: document.body })
  await flushPromises()
  return w
}

function lastEmitted(w: Awaited<ReturnType<typeof mountEditor>>): string {
  const all = w.emitted('update:html') ?? []
  return (all[all.length - 1]?.[0] as string | undefined) ?? ''
}

describe('RichTextEditor', () => {
  test('署名ブロック・引用ブロックを属性ごと保持する', async () => {
    const html = `<p>本文</p>${buildSignatureBlock('acc-1', 'sig')}${QUOTE}`
    const w = await mountEditor(html)
    const root = w.get('.editor-content').element
    expect(
      root.querySelector('div[data-hitomado-signature][data-account-id="acc-1"]'),
    ).not.toBeNull()
    expect(root.querySelector('blockquote[data-hitomado-quote]')).not.toBeNull()
    w.unmount()
  })

  test('編集後の出力にも署名（アカウント ID 付き）と引用が残り、順序が保たれる', async () => {
    const html = `<p>本文</p>${buildSignatureBlock('acc-1', 'sig')}${QUOTE}`
    const w = await mountEditor(html)
    // 先頭の段落末尾へ入力する（ProseMirror の DOM 操作を介さず、公開コマンドで編集する）
    const ed = w.findComponent({ name: 'EditorContent' }).props('editor') as Editor
    ed.chain().focus('start').insertContent('追記').run()
    await flushPromises()
    const out = lastEmitted(w)
    expect(out).toContain('追記')
    expect(findSignature(out)?.accountId).toBe('acc-1')
    expect(out.indexOf('data-hitomado-signature')).toBeLessThan(out.indexOf('data-hitomado-quote'))
    w.unmount()
  })

  test('許可外の書式（見出し・斜体・番号付きリスト等）はスキーマで落ちる', async () => {
    const w = await mountEditor(
      '<h1>見出し</h1><p><em>斜体</em><strong>太字</strong></p><ol><li>一</li></ol>',
    )
    const root = w.get('.editor-content').element
    expect(root.querySelector('h1')).toBeNull()
    expect(root.querySelector('em')).toBeNull()
    expect(root.querySelector('ol')).toBeNull()
    expect(root.querySelector('strong')).not.toBeNull()
    w.unmount()
  })

  test('http／https／mailto 以外のリンクは取り込まない', async () => {
    const w = await mountEditor(
      '<p><a href="javascript:alert(1)">悪</a><a href="https://example.com">良</a></p>',
    )
    const hrefs = Array.from(w.get('.editor-content').element.querySelectorAll('a')).map((a) =>
      a.getAttribute('href'),
    )
    expect(hrefs).toEqual(['https://example.com'])
    w.unmount()
  })

  test('props.html の外部変更はエディタへ反映され、update を発行しない', async () => {
    const w = await mountEditor('<p>a</p>')
    await w.setProps({ html: '<p>別の本文</p>' })
    await flushPromises()
    expect(w.get('.editor-content').text()).toContain('別の本文')
    expect(w.emitted('update:html')).toBeUndefined()
    w.unmount()
  })

  test('editable=false ではツールバーを出さない', async () => {
    const w = mount(RichTextEditor, { props: { html: '<p>a</p>', editable: false } })
    await flushPromises()
    expect(w.find('[role="toolbar"]').exists()).toBe(false)
    w.unmount()
  })

  test('Ctrl+Enter は本文に改行を入れず、キーイベントは上位へ伝わる（送信確認用）', async () => {
    const w = await mountEditor('<p>本文</p>')
    const ed = w.findComponent({ name: 'EditorContent' }).props('editor') as Editor
    ed.commands.focus('end')
    let reached = false
    document.body.addEventListener('keydown', () => {
      reached = true
    })
    const ev = new KeyboardEvent('keydown', {
      key: 'Enter',
      ctrlKey: true,
      bubbles: true,
      cancelable: true,
    })
    ed.view.dom.dispatchEvent(ev)
    expect(reached).toBe(true)
    expect(ed.getHTML()).not.toContain('<br')
    w.unmount()
  })
})
