// Ousiass デザインガイド（brand-guide a66e8514）の反映の回帰テスト（#248・#264・#265・#283・#285・#306）。
import { beforeEach, describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'
import { parse } from '@vue/compiler-sfc'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import AiDraftButton from '../../src/components/compose/ai/AiDraftButton.vue'
import { useSettingsStore } from '../../src/stores/settings'
import { rule } from './styleRule'

beforeEach(() => {
  document.body.innerHTML = ''
  setActivePinia(createPinia())
})

function sfcStyle(file: string): string {
  return parse(readFileSync(new URL(`../../src/components/${file}`, import.meta.url), 'utf8'))
    .descriptor.styles[0].content
}

describe('フォーカスは --accent の outline（--ring・--focus・box-shadow をやめる。#248・#389）', () => {
  // 入力系 2px・offset −1px（枠を --accent にして 2px の線で囲う）。出すのは :focus-visible のときだけ
  test.each([
    ['settings/SettingsInput.vue', '.field-input:focus-visible {'],
    ['ui/BaseSelect.vue', '.sel:focus-visible {'],
    ['inbox/SearchBar.vue', '.input:focus-visible {'],
    ['compose/ComposeFields.vue', '.subject-input:focus-visible {'],
    ['compose/recipient/RecipientField.vue', '.recipient-box:has(:focus-visible) {'],
    ['compose/EditorLinkForm.vue', '.rte-link-input:focus-visible {'],
    ['compose/ai/AiDraftInstruction.vue', '.instruction-input:focus-visible {'],
    ['thread/ai/ThreadAiQuestionForm.vue', '.question-input:focus-visible {'],
    ['account/AddAccountDialog.vue', '.text:focus-visible {'],
  ])('入力系 %s は 2px・offset -1px', (file, selector) => {
    const r = rule(file, selector)
    expect(r).toContain('border-color: var(--accent)')
    expect(r).toContain('outline: 2px solid var(--accent)')
    expect(r).toContain('outline-offset: -1px')
    expect(r).not.toContain('box-shadow')
    expect(r).not.toContain('outline: none')
  })

  // 本文エディタは太いリングを出さず、1px の枠線の色を --accent に変えるだけ（:focus-visible のときだけ）
  test('本文エディタの外枠は :focus-visible のとき、枠線（1px のまま）の色だけ --accent にする', () => {
    const r = rule('compose/RichTextEditor.vue', '.rte:has(:focus-visible) {')
    expect(r).toContain('border-color: var(--accent)')
    expect(r).not.toMatch(/outline|box-shadow|border-width|border:/)
    expect(rule('compose/RichTextEditor.vue', '.rte {')).toContain(
      'border: 1px solid var(--border)',
    )
    expect(sfcStyle('compose/RichTextEditor.vue')).not.toContain(':focus-within')
  })

  // 離散コントロール 3px・offset 2px
  test.each([
    ['ui/BaseSwitch.vue', '.sw:focus-visible {'],
    ['settings/ai/ChatGPTAccountRow.vue', '.radio:focus-visible {'],
  ])('離散コントロール %s は 3px・offset 2px', (file, selector) => {
    const r = rule(file, selector)
    expect(r).toContain('outline: 3px solid var(--accent)')
    expect(r).toContain('outline-offset: 2px')
    expect(r).not.toContain('box-shadow')
  })
})

describe('入力欄の文字サイズ（#283）', () => {
  test.each([
    ['compose/recipient/RecipientField.vue', '.recipient-input {'],
    ['compose/ComposeFields.vue', '.subject-input {'],
    ['thread/ai/ThreadAiQuestionForm.vue', '.question-input {'],
    ['compose/ai/AiDraftInstruction.vue', '.instruction-input {'],
  ])('%s は base 16', (file, selector) => {
    expect(rule(file, selector)).toContain('font-size: var(--font-size-base)')
  })

  test.each([
    ['inbox/SearchBar.vue', '.input'],
    ['settings/SettingsInput.vue', '.field-input'],
    ['ui/BaseSelect.vue', '.sel'],
    ['compose/EditorLinkForm.vue', '.rte-link-input'],
    ['account/AddAccountDialog.vue', '.text'],
  ])('%s（%s）はデスクトップで sm 14・640px 以下で 16', (file, cls) => {
    expect(rule(file, `${cls} {`)).toContain('font-size: var(--font-size-sm)')
    expect(sfcStyle(file)).toMatch(
      new RegExp(
        `@media \\(max-width: 640px\\) \\{\\s*\\${cls} \\{\\s*font-size: var\\(--font-size-base\\);`,
      ),
    )
  })
})

describe('個別の値（#265・#285・#306）', () => {
  test('MessageCard のトグルと本文の左右 padding は --pad-card-sm', () => {
    expect(rule('thread/MessageCard.vue', '.card-toggle {')).toContain(
      'padding: var(--s-2) var(--pad-card-sm)',
    )
    expect(rule('thread/MessageCard.vue', '.card-content {')).toContain(
      'padding: 0 var(--pad-card-sm) var(--s-3)',
    )
  })

  test('BaseMenu の使えない項目は opacity .4', () => {
    expect(rule('ui/BaseMenu.vue', '.menu-item[data-disabled] {')).toContain('opacity: 0.4')
  })

  test('BaseMenu の強調行は --accent-soft の地だけで、キーボードで強調してもリング（outline・box-shadow）を足さない', () => {
    const css = sfcStyle('ui/BaseMenu.vue').replace(/\/\*[\s\S]*?\*\//g, '')
    expect(rule('ui/BaseMenu.vue', '.menu-item[data-highlighted] {')).toContain(
      'background: var(--accent-soft)',
    )
    expect(rule('ui/BaseMenu.vue', '.menu-item {')).toContain('outline: none')
    expect(css).not.toContain(':focus-visible')
    expect(css).not.toContain('box-shadow')
    expect(css.match(/outline/g)).toHaveLength(1)
  })

  test('トグルの押下（.rte-btn-on）は --accent-soft の地と --ink の文字だけで、枠を足さない', () => {
    const r = rule('compose/RichTextEditor.vue', '.rte-toolbar :deep(.rte-btn-on) {')
    expect(r).toContain('background: var(--accent-soft)')
    expect(r).toContain('color: var(--ink)')
    expect(r).not.toMatch(/border/)
  })

  test('ThreadAiPanel の .mute-heading は sm・行間 sm・600・色は --mute のまま', () => {
    const r = rule('thread/ai/ThreadAiPanel.vue', '.mute-heading {')
    expect(r).toContain('font-size: var(--font-size-sm)')
    expect(r).toContain('line-height: var(--line-height-sm)')
    expect(r).toContain('font-weight: 600')
    expect(r).toContain('color: var(--mute)')
  })
})

describe('AiDraftButton の無効の理由（#264）', () => {
  test('キー未設定のとき、ボタンに title を付けず、aria-describedby で見えている .ai-note につなぐ', () => {
    const store = useSettingsStore()
    store.aiConfig = {
      features: {
        draft: { provider: 'openai', model: 'gpt' },
        threadAi: { provider: '', model: '' },
        inboxZero: { provider: '', model: '' },
      },
      apiKeyProviders: [],
      consentedProviders: [],
      chatgptSignedIn: false,
      chatgptAccounts: [],
    }
    const w = mount(AiDraftButton, { props: { sessionId: 's1' }, attachTo: document.body })
    const button = w.get('button.btn')
    const note = w.get('.ai-note')
    expect(button.attributes('disabled')).toBeDefined()
    expect(button.attributes('title')).toBeUndefined()
    expect(note.attributes('id')).toBeTruthy()
    expect(button.attributes('aria-describedby')).toBe(note.attributes('id'))
  })

  test('使えるときは aria-describedby も .ai-note も出さない', () => {
    const store = useSettingsStore()
    store.aiConfig = {
      features: {
        draft: { provider: 'openai', model: 'gpt' },
        threadAi: { provider: '', model: '' },
        inboxZero: { provider: '', model: '' },
      },
      apiKeyProviders: ['openai'],
      consentedProviders: ['openai'],
      chatgptSignedIn: false,
      chatgptAccounts: [],
    }
    const w = mount(AiDraftButton, { props: { sessionId: 's1' }, attachTo: document.body })
    expect(w.find('.ai-note').exists()).toBe(false)
    expect(w.get('button.btn').attributes('aria-describedby')).toBeUndefined()
  })
})
