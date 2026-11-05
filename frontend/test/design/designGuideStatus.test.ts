// Ousiass デザインガイド準拠の回帰テスト（#129〜）。状態の伝え方（C4）とフォーカス（C3）の規則を確かめる。
import { beforeEach, describe, expect, test } from 'bun:test'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import type { ThreadSummary } from '../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import UsageLimitNotice from '../../src/components/compose/ai/UsageLimitNotice.vue'
import ComposePane from '../../src/components/compose/ComposePane.vue'
import RecipientField from '../../src/components/compose/RecipientField.vue'
import ThreadRow from '../../src/components/inbox/ThreadRow.vue'
import AIDestinationBox from '../../src/components/settings/ai/AIDestinationBox.vue'
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
