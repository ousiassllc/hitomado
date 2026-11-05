// Ousiass デザインガイド準拠の回帰テスト（#137）。形・余白・線（S1）の規則を確かめる。
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'

const style = (rel: string) => {
  const src = readFileSync(new URL(`../../src/components/${rel}`, import.meta.url), 'utf8')
  return /<style[\s\S]*<\/style>/.exec(src)?.[0] ?? ''
}
const radius = (css: string, selector: string) =>
  new RegExp(`${selector.replace('.', '\\.')}\\s*\\{([^}]*)\\}`)
    .exec(css)?.[1]
    .match(/border-radius:\s*([^;]+);/)?.[1]

describe('入れ子のパネルは --bone + --r-md（S1・#137）', () => {
  const panels: [string, string, string][] = [
    ['account/SenderBadge.vue', '.sender-badge', '--r-md'],
    ['settings/McpSettings.vue', '.token-box', '--r-md'],
    ['settings/accounts/AccountRow.vue', '.row', '--r-md'],
    ['settings/ai/AIDestinationBox.vue', '.dest', '--r-md'],
    // 添付はタグ相当。作成画面の AttachmentBar と同じ --r-sm
    ['thread/AttachmentList.vue', '.attachment', '--r-sm'],
    ['settings/oauth/OAuthClientForm.vue', '.form', '--r-md'],
  ]
  test.each(panels)('%s の %s は var(%s)', (file, selector, token) => {
    const css = style(file)
    const rule = radius(css, selector)
    expect(rule).toBe(`var(${token})`)
    expect(
      new RegExp(`${selector.replace('.', '\\.')}\\s*\\{[^}]*background: var\\(--bone\\)`).test(
        css,
      ),
    ).toBe(true)
  })
})
