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

describe('パネル・通知の角丸は --r-md（S1・#143）', () => {
  // --r-sm はタグ・チェック用。枠つきの警告・通知・一覧は入れ子パネルと同じ --r-md。
  test.each([
    ['compose/SendConfirmDialog.vue', '.warning'],
    ['compose/ai/UsageLimitNotice.vue', '.limit'],
    ['settings/ai/AIChatGPTLogin.vue', '.accounts'],
  ])('%s の %s は var(--r-md)', (file, selector) => {
    expect(radius(style(file), selector)).toBe('var(--r-md)')
  })
})

describe('フォームの間隔レシピ（S2・#143）', () => {
  const tokens = readFileSync(new URL('../../src/styles/tokens.css', import.meta.url), 'utf8')
  const gapOf = (css: string, selector: string) =>
    new RegExp(`${selector.replace('.', '\\.')}\\s*\\{([^}]*)\\}`)
      .exec(css)?.[1]
      .match(/gap:\s*([^;]+);/)?.[1]

  test('ラベル→入力は 8px、入力→補足は 4px、項目→項目は 16px のトークンを持つ', () => {
    expect(tokens).toContain('--gap-label: var(--s-2);')
    expect(tokens).toContain('--gap-help: var(--s-1);')
    expect(tokens).toContain('--gap-field: var(--s-4);')
  })

  test.each([
    ['settings/SettingsInput.vue', '.field'],
    ['settings/ai/AISettings.vue', '.field'],
    ['settings/ai/AIModelField.vue', '.field'],
    ['settings/ai/AIEffortField.vue', '.field'],
    ['account/AddAccountDialog.vue', '.field'],
  ])('%s の %s はラベルと入力の間を --gap-label にする', (file, selector) => {
    expect(gapOf(style(file), selector)).toBe('var(--gap-label)')
  })

  test.each([
    ['settings/ai/AISettings.vue', '.form'],
    ['settings/oauth/OAuthClientForm.vue', '.form'],
    ['settings/ai/AIApiKey.vue', '.key'],
    ['account/AddAccountDialog.vue', '.stack'],
  ])('%s の %s は項目と項目の間を --gap-field にする', (file, selector) => {
    expect(gapOf(style(file), selector)).toBe('var(--gap-field)')
  })

  test.each([
    ['settings/SettingsInput.vue', '.field-hint'],
    ['settings/SettingsInput.vue', '.field-error'],
    ['settings/ai/AIModelField.vue', '.error'],
    ['settings/ai/AIEffortField.vue', '.hint'],
  ])('%s の %s は補足を --gap-help まで入力へ寄せる', (file, selector) => {
    expect(
      new RegExp(
        `${selector.replace('.', '\\.')}\\s*\\{[^}]*margin:\\s*calc\\(var\\(--gap-help\\) - var\\(--gap-label\\)\\) 0 0;`,
      ).test(style(file)),
    ).toBe(true)
  })
})

const body = (file: string, selector: string) =>
  new RegExp(`${selector.replace('.', '\\.')}\\s*\\{([^}]*)\\}`).exec(style(file))?.[1] ?? ''

describe('宛先の候補一覧・作成画面の間隔（S2・#150）', () => {
  test('候補一覧は BaseMenu と同じ寸法（panel padding 4・項目 40・左右 --menu-pad-x・--r-sm）', () => {
    const file = 'compose/RecipientField.vue'
    expect(body(file, '.suggest')).toContain('padding: var(--s-1);')
    const item = body(file, '.suggest-item')
    expect(item).toContain('min-height: var(--menu-item-h);')
    expect(item).toContain('padding: 0 var(--menu-pad-x);')
    expect(item).toContain('border-radius: var(--r-sm);')
    expect(item).toContain('align-items: center;')
  })

  test('作成画面の項目と項目の間は --gap-field', () => {
    expect(body('compose/ComposeFields.vue', '.compose-fields')).toContain('gap: var(--gap-field);')
  })
})

describe('空状態は破線の枠・アイコン・文言（S4・#150）', () => {
  test('EmptyState は 1px dashed --border・--r-lg・padding 48/24', () => {
    const css = body('ui/EmptyState.vue', '.empty-state')
    expect(css).toContain('border: 1px dashed var(--border);')
    expect(css).toContain('border-radius: var(--r-lg);')
    expect(css).toContain('padding: var(--s-12) var(--s-6);')
  })

  test.each([
    'inbox/ThreadList.vue',
    'thread/ThreadPane.vue',
    'account/AccountList.vue',
    'settings/SignatureSettings.vue',
    'settings/accounts/AccountsSettings.vue',
    'settings/oauth/OAuthClientsSettings.vue',
  ])('%s は空状態に EmptyState を使う', (file) => {
    const src = readFileSync(new URL(`../../src/components/${file}`, import.meta.url), 'utf8')
    expect(src).toMatch(/import EmptyState from '(\.\.\/)+ui\/EmptyState\.vue'/)
    expect(src).toContain('<EmptyState')
  })
})
