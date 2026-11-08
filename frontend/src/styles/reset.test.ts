import { describe, expect, test } from 'bun:test'

// ガイド §2: 見出しの 600 は役割クラスに置き、h1〜h6 の要素セレクタには置かない（タグを差し替えても見た目が変わらない）。
const resetCss = await Bun.file(new URL('./reset.css', import.meta.url)).text()

// 見出しタグ（h1〜h6）に付ける役割クラス。[コンポーネントのパス, クラス名]
const headingClasses: [string, string][] = [
  ['../components/account/AddAccountDialog.vue', 'title'],
  ['../components/compose/ComposePane.vue', 'compose-title'],
  ['../components/compose/ai/UsageLimitNotice.vue', 'title'],
  ['../components/settings/GeneralSettings.vue', 'block-title'],
  ['../components/settings/McpSettings.vue', 'heading'],
  ['../components/settings/ai/AIApiKey.vue', 'sub'],
  ['../components/settings/ai/AIChatGPTLogin.vue', 'sub'],
  ['../components/settings/ai/AIConsentSection.vue', 'sub'],
  ['../components/settings/ai/AISettings.vue', 'heading'],
  ['../components/settings/oauth/OAuthClientForm.vue', 'title'],
  ['../components/thread/ai/ThreadAiPanel.vue', 'heading'],
  ['../components/thread/ai/ThreadAiPanel.vue', 'mute-heading'],
  ['../components/thread/ThreadHeader.vue', 'subject'],
  ['../components/ui/BaseDialog.vue', 'dlg-title'],
]

describe('見出しの font-weight', () => {
  test('reset.css は h1〜h6 の要素セレクタに font-weight を置かない', () => {
    expect(resetCss).not.toMatch(/\bh[1-6]\b[^{}]*\{[^}]*font-weight/)
  })

  test.each(headingClasses)('%s の .%s は font-weight: 600', async (path, cls) => {
    const sfc = await Bun.file(new URL(path, import.meta.url)).text()
    const rule = sfc.match(new RegExp(`\\n\\.${cls} \\{([^}]*)\\}`))
    expect(rule?.[1]).toMatch(/font-weight:\s*600;/)
  })
})
