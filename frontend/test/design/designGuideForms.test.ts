// Ousiass デザインガイド準拠の回帰テスト（#125〜#128）。入力欄・文字・部品の見た目の規則を、スタイルの記述で確かめる。
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'
import { mount } from '@vue/test-utils'
import SearchBar from '../../src/components/inbox/SearchBar.vue'

const read = (rel: string) => readFileSync(new URL(rel, import.meta.url), 'utf8')

describe('フォームの focus（C3・#125）', () => {
  // 入力欄の focus は --focus の枠線 + --ring の 2px。外枠を持つもの（宛先・本文）は :focus-within で同じ指定にする。
  const focusRule = (rel: string, selector: string) => {
    const style = /<style[\s\S]*<\/style>/.exec(read(`../../src/components/${rel}`))?.[0] ?? ''
    const escaped = selector.replace(/[.:]/g, '\\$&')
    return new RegExp(`${escaped}\\s*\\{([^}]*)\\}`).exec(style)?.[1] ?? ''
  }

  test.each([
    ['compose/ComposeFields.vue', '.subject-input:focus'],
    ['compose/ai/AiDraftInstruction.vue', '.instruction-input:focus'],
    ['compose/RecipientField.vue', '.recipient-box:focus-within'],
    ['compose/RichTextEditor.vue', '.rte:focus-within'],
    ['settings/SettingsInput.vue', '.field-input:focus'],
    ['inbox/SearchBar.vue', '.input:focus'],
  ])('%s の %s は --focus の枠線と --ring の 2px を使う', (rel, selector) => {
    const body = focusRule(rel, selector)
    expect(body).toContain('border-color: var(--focus)')
    expect(body).toContain('box-shadow: 0 0 0 2px var(--ring)')
  })
})

describe('タイポグラフィ（T・#126）', () => {
  const reset = read('../../src/styles/reset.css')
  const rule = (css: string, selector: string) =>
    new RegExp(`${selector}\\s*\\{([^}]*)\\}`).exec(css)?.[1] ?? ''

  test('見出し h1〜h6 は SemiBold（600）、見出しでない <strong> は 400', () => {
    expect(rule(reset, 'h1,\\s*h2,\\s*h3,\\s*h4,\\s*h5,\\s*h6')).toContain('font-weight: 600')
    expect(rule(reset, 'strong')).toContain('font-weight: 400')
  })

  test('本文エディタで利用者が付けた太字は 400 に負けず太字で見える', () => {
    const style = read('../../src/components/compose/RichTextEditor.vue')
    expect(rule(style, '\\.rte-body :deep\\(\\.editor-content strong\\)')).toContain(
      'font-weight: 700',
    )
  })

  test.each([
    ['settings/McpSettings.vue', '\\.token'],
    ['settings/McpSettings.vue', '\\.path'],
    ['settings/oauth/OAuthClientsSettings.vue', '\\.client-id'],
  ])('%s の %s（識別子・トークン）は JetBrains Mono（--font-mono）', (rel, selector) => {
    expect(rule(read(`../../src/components/${rel}`), selector)).toContain(
      'font-family: var(--font-mono)',
    )
  })
})

describe('入力欄の padding（S2・#127）', () => {
  const padding = (rel: string, selector: string) => {
    const style = /<style[\s\S]*<\/style>/.exec(read(`../../src/components/${rel}`))?.[0] ?? ''
    return new RegExp(`${selector}\\s*\\{[^}]*?padding:\\s*([^;]+);`).exec(style)?.[1]
  }

  test.each([
    ['inbox/SearchBar.vue', '\\.input', '0 var(--s-3)'],
    ['settings/SettingsInput.vue', '\\.field-input', '0 var(--s-3)'],
    ['compose/ComposeFields.vue', '\\.subject-input', '0 var(--s-3)'],
    ['compose/ai/AiDraftInstruction.vue', '\\.instruction-input', 'var(--s-3)'],
    ['compose/RecipientField.vue', '\\.recipient-box', 'var(--s-1) var(--s-3)'],
  ])('%s の %s は左右 12px（--s-3）で揃える', (rel, selector, expected) => {
    expect(padding(rel, selector)).toBe(expected)
  })
})

describe('部品・オーバーレイ（P・#128）', () => {
  const style = (rel: string) =>
    /<style[\s\S]*<\/style>/.exec(read(`../../src/components/${rel}`))?.[0] ?? ''
  const rule = (css: string, selector: string) =>
    new RegExp(`${selector}\\s*\\{([^}]*)\\}`).exec(css)?.[1] ?? ''

  test('検索欄は左に検索アイコン、入力欄の左 padding はアイコン分広い', () => {
    const w = mount(SearchBar, { props: { active: false, query: '' } })
    expect(w.get('.field > svg').attributes('aria-hidden')).toBe('true')
    expect(w.find('.field > input').exists()).toBe(true)
    expect(rule(style('inbox/SearchBar.vue'), '\\.input')).toContain(
      'padding-left: calc(var(--s-3) + var(--icon-md) + var(--s-2))',
    )
  })

  test.each([
    ['compose/RecipientField.vue', '\\.recipient-disabled'],
    ['compose/RichTextEditor.vue', '\\.rte-disabled'],
    ['compose/ComposeFields.vue', '\\.subject-input:disabled'],
    ['settings/SettingsInput.vue', '\\.field-input:disabled'],
  ])('%s の %s は opacity 0.5', (rel, selector) => {
    expect(rule(style(rel), selector)).toContain('opacity: 0.5;')
  })

  test('ドロップダウンは min-width 180、ツールチップは padding 6×10', () => {
    expect(rule(style('ui/BaseMenu.vue'), ':global\\(\\.menu-pop\\)')).toContain(
      'min-width: 180px;',
    )
    expect(rule(style('ui/BaseTooltip.vue'), ':global\\(\\.tip-pop\\)')).toContain(
      'padding: 6px 10px;',
    )
  })
})
