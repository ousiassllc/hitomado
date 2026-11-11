// Ousiass デザインガイド準拠の回帰テスト（#129〜）。状態の伝え方（C4）とフォーカス（C3）の規則を確かめる。
import { beforeEach, describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'
import { parse } from '@vue/compiler-sfc'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import type { ThreadSummary } from '../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import UsageLimitNotice from '../../src/components/compose/ai/UsageLimitNotice.vue'
import ComposePane from '../../src/components/compose/ComposePane.vue'
import RecipientField from '../../src/components/compose/recipient/RecipientField.vue'
import ThreadRow from '../../src/components/inbox/ThreadRow.vue'
import AIDestinationBox from '../../src/components/settings/ai/AIDestinationBox.vue'
import ChatGPTAccountRow from '../../src/components/settings/ai/ChatGPTAccountRow.vue'
import BannerStack from '../../src/components/shell/BannerStack.vue'
import MessageBody from '../../src/components/thread/MessageBody.vue'
import { useComposeStore } from '../../src/stores/compose'
import { useUiStore } from '../../src/stores/ui'

beforeEach(() => {
  document.body.innerHTML = ''
  setActivePinia(createPinia())
})

describe('要再接続の行は濃淡だけで伝えない（C4・#129）', () => {
  const thread: ThreadSummary = {
    accountId: 'a1',
    threadId: 't1',
    subject: '見積のご確認',
    snippet: 'お世話になっております',
    lastMessageAt: Date.now(),
    lastFrom: { name: '山田太郎', email: 'yamada@example.com' },
    messageCount: 1,
    isUnread: true,
    hasAttachment: false,
    inInbox: true,
    labels: [],
  }
  const account = { displayName: '会社A', email: 'a@example.com', color: 'teal' }

  test('アイコンと文言で伝え、aria-label にも含める（title だけにしない）', () => {
    const w = mount(ThreadRow, {
      props: { thread, selected: false, account: { ...account, status: 'needs_reauth' } },
    })
    const note = w.get('.reauth')
    expect(note.text()).toBe('再接続が必要なアカウントです')
    expect(note.find('svg').exists()).toBe(true)
    expect(w.attributes('aria-label')).toContain('再接続が必要なアカウントです')
    expect(w.attributes('title')).toBeUndefined()
  })

  test('通常のアカウントの行には出さない', () => {
    const w = mount(ThreadRow, {
      props: { thread, selected: false, account: { ...account, status: 'active' } },
    })
    expect(w.find('.reauth').exists()).toBe(false)
    expect(w.attributes('aria-label')).not.toContain('再接続')
  })
})

describe('一覧の行の既読の送信者の色（#259）', () => {
  const style = parse(
    readFileSync(new URL('../../src/components/inbox/ThreadRow.vue', import.meta.url), 'utf8'),
  ).descriptor.styles[0].content

  test('既読は --row-mute（ホバー・選択・複数選択では --mute-on-soft になる補助文字の色）にする', () => {
    expect(style).toMatch(/\n\.sender \{\s*color: var\(--row-mute\);\s*\}/)
    expect(style).toMatch(/\.row \{[^}]*--row-mute: var\(--mute\);/)
    expect(style).toMatch(
      /\.row:hover,\s*\.row\.selected,\s*\.row\.checked \{\s*--row-mute: var\(--mute-on-soft\);/,
    )
  })

  test('未読の送信者は --ink のままで、--row-mute にしない', () => {
    expect(style).toMatch(/\.unread \.sender,\s*\.unread \.subject \{\s*color: var\(--ink\);\s*\}/)
  })

  test('要再接続の行の差出人も --row-mute にし、通常時は --mute のまま、ホバー・選択では --mute-on-soft になる（AA）', () => {
    // .row.needs-reauth .sender は .sender より詳細度が高いので、--mute 直書きだとホバー・選択で --mute-on-soft に切り替わらない
    expect(style).toMatch(/\.row\.needs-reauth \.sender \{\s*color: var\(--row-mute\);\s*\}/)
    expect(style).not.toMatch(/\.row\.needs-reauth \.sender[^{]*\{[^}]*color: var\(--mute\)/)
    // 通常時の --row-mute は --mute（見た目は変わらない）で、ホバー・選択・複数選択で --mute-on-soft
    expect(style).toMatch(/\.row \{[^}]*--row-mute: var\(--mute\);/)
    expect(style).toMatch(
      /\.row:hover,\s*\.row\.selected,\s*\.row\.checked \{\s*--row-mute: var\(--mute-on-soft\);/,
    )
  })

  test('要再接続の行の件名も --row-mute にし、通常時は --mute のまま、ホバー・選択では --mute-on-soft になる（AA）', () => {
    expect(style).toMatch(/\.row\.needs-reauth \.subject \{\s*color: var\(--row-mute\);\s*\}/)
    expect(style).not.toMatch(/\.row\.needs-reauth \.subject[^{]*\{[^}]*color: var\(--mute\)/)
  })
})

describe('無効な宛先（C4・#129）', () => {
  test('形式が正しくないアドレスは、チップの警告アイコンと入力欄の下の文言で伝える', () => {
    const w = mount(RecipientField, {
      props: { label: '宛先', modelValue: [{ email: 'ok@example.com' }, { email: 'broken' }] },
    })
    const chips = w.findAll('.chip')
    expect(chips[0].find('.chip-icon').exists()).toBe(false)
    expect(chips[1].find('.chip-icon').exists()).toBe(true)
    const note = w.get('.recipient-invalid')
    expect(note.text()).toContain('形式が正しくないアドレスです')
    expect(note.find('svg').exists()).toBe(true)
    expect(w.get('input').attributes('aria-describedby')).toBe(note.attributes('id'))
  })

  test('すべて有効なら警告を出さない', () => {
    const w = mount(RecipientField, {
      props: { label: '宛先', modelValue: [{ email: 'ok@example.com' }] },
    })
    expect(w.find('.chip-icon').exists()).toBe(false)
    expect(w.find('.recipient-invalid').exists()).toBe(false)
    expect(w.get('input').attributes('aria-describedby')).toBeUndefined()
  })
})

describe('バナー（C4・#129）', () => {
  test('状態のアイコン＋文言で出す', () => {
    useUiStore().online = false
    const w = mount(BannerStack)
    const banner = w.get('.banner[role="status"]')
    expect(banner.find('svg').exists()).toBe(true)
    expect(banner.text()).toContain('オフラインです')
  })
})

describe('作成画面のエラー（C4・#129）', () => {
  async function mountPane() {
    const store = useComposeStore()
    store.session = {
      sessionId: 's1',
      mode: 'new',
      senderLocked: false,
      content: { to: [], cc: null, bcc: null, subject: '', bodyHtml: '' },
      attachments: [],
    }
    store.content = { to: [], cc: [], bcc: [], subject: '', bodyHtml: '' }
    const w = mount(ComposePane, { attachTo: document.body })
    await flushPromises()
    return { w, store }
  }

  test('送信エラー・下書き保存エラーは、アイコン＋見出し＋本文のアラートで出す', async () => {
    const { w, store } = await mountPane()
    store.error = '送信に失敗しました'
    store.saveError = 'ディスクがいっぱいです'
    await flushPromises()
    const alerts = w.findAll('.compose-error[role="alert"]')
    expect(alerts).toHaveLength(2)
    for (const a of alerts) {
      expect(a.find('svg').exists()).toBe(true)
      expect(a.text()).toContain('うまくいきませんでした')
    }
    expect(alerts[0].text()).toContain('送信に失敗しました')
    expect(alerts[1].text()).toContain('ディスクがいっぱいです')
    w.unmount()
  })

  test('エラーが無いときはアラートを出さない', async () => {
    const { w } = await mountPane()
    expect(w.find('.compose-error').exists()).toBe(false)
    w.unmount()
  })
})

describe('警告・エラーの案内はアイコンを付ける（C4・#135）', () => {
  test('画像ブロックの案内は、警告アイコン＋文言を出す', () => {
    const w = mount(MessageBody, { props: { html: '<p>x</p>', hasRemoteImages: true } })
    const bar = w.get('[data-testid="image-bar"]')
    expect(bar.find('svg').exists()).toBe(true)
    expect(bar.text()).toContain('画像')
    expect(bar.get('svg').classes()).toContain('icon-warning')
  })

  test('送信先の「保存前」の警告は、警告アイコン＋文言を出す', () => {
    const w = mount(AIDestinationBox, {
      props: { provider: 'anthropic', draftProvider: 'openai', draftBaseUrl: '' },
    })
    const note = w.get('.dest-pending')
    expect(note.find('svg').exists()).toBe(true)
    expect(note.text()).not.toBe('')
    expect(note.get('svg').classes()).toContain('icon-warning')
  })
})

describe('利用枠超過の案内は状態色の部品（C4・#141）', () => {
  test('警告アイコン＋文言を role=alert で出す', () => {
    const w = mount(UsageLimitNotice)
    const notice = w.get('[data-testid="ai-usage-limit"]')
    expect(notice.attributes('role')).toBe('alert')
    expect(notice.get('svg').classes()).toContain('icon-warning')
    expect(notice.get('svg').attributes('aria-hidden')).toBe('true')
  })
})

describe('ChatGPT アカウント行のステータスの札は状態色のピル（C4・#178）', () => {
  const account = {
    id: 'c1',
    email: 'a@example.com',
    signedIn: true,
    active: true,
    planNoticePending: false,
  }
  const props = { label: 'a@example.com', busy: false, radioDisabled: false, loggingOut: false }

  test('使用中は success、ログアウト済みは warning の札にする', () => {
    const active = mount(ChatGPTAccountRow, { props: { ...props, account } })
    expect(active.get('[data-testid="ai-chatgpt-active"]').classes()).toEqual([
      'badge',
      'badge-active',
    ])
    const out = mount(ChatGPTAccountRow, {
      props: { ...props, account: { ...account, signedIn: false, active: false } },
    })
    expect(out.get('.badge').classes()).toContain('badge-signed-out')
  })

  test('札のスタイルは状態色のトークンと --r-full を使う', () => {
    const style = parse(
      readFileSync(
        new URL('../../src/components/settings/ai/ChatGPTAccountRow.vue', import.meta.url),
        'utf8',
      ),
    ).descriptor.styles[0].content
    expect(style).toContain('border-radius: var(--r-full)')
    expect(style).toContain('--badge-color: var(--success)')
    expect(style).toContain('--badge-border: var(--success-border)')
    expect(style).toContain('--badge-soft: var(--success-soft)')
    expect(style).toContain('--badge-color: var(--warning)')
    expect(style).toContain('--badge-border: var(--warning-border)')
    expect(style).toContain('--badge-soft: var(--warning-soft)')
  })
})

describe('ChatGPT アカウント行のラジオはガイドの見た目（O8・#181）', () => {
  const style = parse(
    readFileSync(
      new URL('../../src/components/settings/ai/ChatGPTAccountRow.vue', import.meta.url),
      'utf8',
    ),
  ).descriptor.styles[0].content

  test('入力に radio クラスを付け、ブラウザ標準の見た目（accent-color）に頼らない', () => {
    const w = mount(ChatGPTAccountRow, {
      props: {
        account: {
          id: 'c1',
          email: 'a@example.com',
          signedIn: true,
          active: true,
          planNoticePending: false,
        },
        label: 'a@example.com',
        busy: false,
        radioDisabled: false,
        loggingOut: false,
      },
    })
    expect(w.get('input[type="radio"]').classes()).toContain('radio')
    expect(style).not.toContain('accent-color')
  })

  test('appearance:none・18px・--mute の外枠・選択時は --accent 地に --bone の点・フォーカスは --focus の 3px outline', () => {
    expect(style).toContain('appearance: none')
    expect(style).toContain('width: 18px')
    expect(style).toContain('height: 18px')
    expect(style).toContain('border: 1px solid var(--mute)')
    expect(style).toContain('background: var(--accent)')
    expect(style).toContain('background: var(--bone)')
    expect(style).toContain('outline: 3px solid var(--focus)')
  })
})

describe('処理中のアイコンボタンは薄めない（O12・#234）', () => {
  test('.icon-btn:disabled の opacity は aria-busy のボタンを除く（BaseButton の loading と同じ）', () => {
    const style = parse(
      readFileSync(new URL('../../src/components/ui/BaseIconButton.vue', import.meta.url), 'utf8'),
    ).descriptor.styles[0].content
    expect(style).toMatch(/\.icon-btn:disabled:not\(\[aria-busy='true'\]\)\s*\{[^}]*opacity: 0\.4/)
    expect(style).not.toMatch(/\.icon-btn:disabled\s*\{/)
  })
})
