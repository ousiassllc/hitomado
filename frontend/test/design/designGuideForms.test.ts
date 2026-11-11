// Ousiass デザインガイド準拠の回帰テスト（#128）。検索欄の構造を確かめる。
import { afterEach, describe, expect, test } from 'bun:test'
import { mount } from '@vue/test-utils'
import { defineComponent, h, nextTick } from 'vue'
import SearchBar from '../../src/components/inbox/SearchBar.vue'
import AIDestinationBox from '../../src/components/settings/ai/AIDestinationBox.vue'
import AISetupDialog from '../../src/components/settings/ai/AISetupDialog.vue'
import { rule } from './styleRule'

describe('部品・オーバーレイ（P・#128）', () => {
  test('検索欄は左に検索アイコンを持つ', () => {
    const w = mount(SearchBar, { props: { active: false, query: '' } })
    expect(w.get('.field > svg').attributes('aria-hidden')).toBe('true')
    expect(w.find('.field > input').exists()).toBe(true)
  })

  test('検索欄はブラウザ標準の✕を消し、右の自前のクリアボタンに一本化する（O8・#229）', () => {
    expect(rule('inbox/SearchBar.vue', '.input::-webkit-search-cancel-button {')).toContain(
      'appearance: none',
    )
  })
})

describe('フォームのグループ見出し（P・#319）', () => {
  test('「データの送信先」は、中身を --surface 地・1px --border・--r-md のパネルに入れる', () => {
    const w = mount(AIDestinationBox, {
      props: { provider: 'openai', draftProvider: 'openai', draftBaseUrl: '' },
    })
    expect(w.find('fieldset').exists()).toBe(false)
    expect(w.find('.dest-panel [data-testid="ai-destination"]').exists()).toBe(true)
    const panel = rule('settings/ai/AIDestinationBox.vue', '.dest-panel {')
    expect(panel).toContain('border: 1px solid var(--border)')
    expect(panel).toContain('border-radius: var(--r-md)')
    expect(panel).toContain('background: var(--surface)')
  })

  test('見出し（h4）とパネルは aria-labelledby でつなぎ、id は部品ごとに一意', () => {
    const props = { provider: 'openai', draftProvider: 'openai', draftBaseUrl: '' }
    // 1 つのアプリに 2 つ並べる（useId はアプリ内で一意）
    const w = mount(
      defineComponent({
        render: () => h('div', [h(AIDestinationBox, props), h(AIDestinationBox, props)]),
      }),
    )
    const headings = w.findAll('h4')
    const panels = w.findAll('section.dest-panel')
    expect(headings).toHaveLength(2)
    for (const [i, heading] of headings.entries()) {
      expect(heading.text()).not.toBe('')
      expect(heading.attributes('id')).toBeTruthy()
      expect(panels[i].attributes('aria-labelledby')).toBe(heading.attributes('id'))
    }
    expect(headings[0].attributes('id')).not.toBe(headings[1].attributes('id'))
  })

  test('見出しは h4 の段（sm・SemiBold）', () => {
    const title = rule('settings/ai/AIDestinationBox.vue', '.dest-title {')
    expect(title).toContain('font-size: var(--font-size-sm)')
    expect(title).toContain('font-weight: 600')
  })
})

afterEach(() => {
  document.body.innerHTML = ''
})

describe('ダイアログのフッター（#318）', () => {
  test('狭い幅ではボタンを折り返し、縮めて切れさせない', () => {
    expect(rule('ui/BaseDialog.vue', '.dlg-foot {')).toContain('flex-wrap: wrap')
  })

  test('AI の初回の案内は md 幅（560px）で、狭い画面では画面幅 - 32px に収まる', async () => {
    const w = mount(AISetupDialog, { props: { open: true }, attachTo: document.body })
    await nextTick()
    expect(document.querySelector('.dlg')?.classList.contains('dlg-md')).toBe(true)
    expect(rule('ui/BaseDialog.vue', '.dlg-md {')).toContain('--dlg-w: 560px')
    expect(rule('ui/BaseDialog.vue', '.dlg {')).toContain(
      'width: min(var(--dlg-w, 520px), calc(100vw - var(--s-8)))',
    )
    w.unmount()
  })
})
