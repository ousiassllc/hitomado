// 390px 幅で、検索プレースホルダと AI 補助文の行が欠けない・食い込まない回帰テスト（#348・#349）。
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'
import { parse } from '@vue/compiler-sfc'
import { rule } from './styleRule'

function style(file: string): string {
  return parse(readFileSync(new URL(`../../src/components/${file}`, import.meta.url), 'utf8'))
    .descriptor.styles[0].content
}

describe('検索プレースホルダ（#348）', () => {
  test('右のクリア分の padding は has-clear のときだけ。入力欄の通常の右 padding は --s-3', () => {
    expect(rule('inbox/SearchBar.vue', '.input {')).toContain('padding: 0 var(--s-3)')
    expect(rule('inbox/SearchBar.vue', '.input {')).not.toContain('padding-right')
    expect(rule('inbox/SearchBar.vue', '.input.has-clear {')).toContain(
      'padding-right: calc(var(--s-1) + var(--control-h-sm) + var(--s-1))',
    )
  })

  test('収まらない幅では「…」で省き、640px 以下ではプレースホルダだけ --font-size-sm にする（入力文字は 16px のまま）', () => {
    expect(rule('inbox/SearchBar.vue', '.input {')).toContain('text-overflow: ellipsis')
    const css = style('inbox/SearchBar.vue')
    const media = css.slice(css.indexOf('@media (max-width: 640px)'))
    expect(media).toContain('font-size: var(--font-size-base)')
    expect(media).toMatch(/\.input::placeholder\s*\{[^}]*font-size: var\(--font-size-sm\)/)
  })
})

describe('タッチ UI の検索（#348）', () => {
  test('右 padding のクリア分（--tap-min）は has-clear のときだけ。常時の上書きでプレースホルダの幅を削らない', () => {
    const css = readFileSync(new URL('../../src/styles/touch.css', import.meta.url), 'utf8')
    const rules = [...css.matchAll(/([^{}]*\.search \.input[^{}]*)\{([^}]*)\}/g)]
    const withPad = rules.filter(([, , body]) => body.includes('padding-right'))
    expect(withPad).toHaveLength(1)
    expect(withPad[0][1]).toContain('.search .input.has-clear')
    expect(withPad[0][2]).toContain('var(--tap-min)')
  })
})

describe('AI 補助文の行（#349）', () => {
  const form = 'thread/ai/ThreadAiQuestionForm.vue'

  test('フッタは折り返せる。補助文は 1 行に要る幅を basis に持ち、足りなければ自分の行に出す', () => {
    expect(rule(form, '.question-foot {')).toContain('flex-wrap: wrap')
    const note = rule(form, '.question-note {')
    expect(note).toContain('flex: 1 1 27em')
    expect(note).toContain('min-width: 0')
  })

  test('補助文は文節の切れ目で折り返し、文字数は折り返さず右に寄せる', () => {
    expect(rule(form, '.question-note {')).toContain('word-break: auto-phrase')
    const count = rule(form, '.question-count {')
    expect(count).toContain('white-space: nowrap')
    expect(count).toContain('margin-left: auto')
  })
})

describe('Inbox Zero の候補ダイアログのボタン（#377）', () => {
  // DOM の順は「やめる」「アーカイブする」（広い幅は右寄せの横並びのまま）。狭い幅だけ縦並びにし、column-reverse で実行を上にする。
  test('stackFooter のダイアログは 640px 以下で、フッターを全幅の縦並び（column-reverse・stretch）にする', () => {
    const css = style('ui/BaseDialog.vue')
    const media = css.slice(css.indexOf('@media (max-width: 640px)'))
    const r = media.match(/\.dlg-foot-stack\s*\{([^}]*)\}/)?.[1] ?? ''
    expect(r).toContain('flex-direction: column-reverse')
    expect(r).toContain('align-items: stretch')
    // 広い幅の .dlg-foot（右寄せ・横並び・折り返し）は変えない
    const foot = rule('ui/BaseDialog.vue', '.dlg-foot {')
    expect(foot).toContain('justify-content: flex-end')
    expect(foot).not.toContain('flex-direction')
  })
})
