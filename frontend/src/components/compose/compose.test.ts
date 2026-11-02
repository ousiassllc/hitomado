import { beforeEach, describe, expect, test } from 'bun:test'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import type {
  ComposeSession,
  SendPreview,
} from '../../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import { useAccountsStore } from '../../stores/accounts'
import { useComposeStore } from '../../stores/compose'
import ComposePane from './ComposePane.vue'
import SendConfirmDialog from './SendConfirmDialog.vue'
import SenderField from './SenderField.vue'

const accounts = [
  { id: 'a1', displayName: '仕事', email: 'work@example.com', color: 'blue' },
  { id: 'a2', displayName: '私用', email: 'me@example.com', color: 'red' },
]

beforeEach(() => {
  document.body.innerHTML = ''
  setActivePinia(createPinia())
})

describe('SenderField', () => {
  test('locked は表示のみ（選択 UI なし）で送信元を常時表示する', () => {
    const w = mount(SenderField, {
      props: { from: { email: 'work@example.com' }, accountId: 'a1', locked: true, accounts },
    })
    expect(w.find('[role="combobox"]').exists()).toBe(false)
    expect(w.find('select').exists()).toBe(false)
    expect(w.find('button').exists()).toBe(false)
    expect(w.text()).toContain('仕事')
    expect(w.text()).toContain('work@example.com')
  })

  test('new は選択式で、未選択状態を示す', () => {
    const w = mount(SenderField, { props: { accountId: undefined, locked: false, accounts } })
    expect(w.find('[role="combobox"]').exists()).toBe(true)
    expect(w.text()).toContain('送信元を選択してください')
  })
})

function newSession(accountId?: string): ComposeSession {
  return {
    sessionId: 's1',
    mode: 'new',
    accountId,
    from: accountId ? { email: 'work@example.com' } : undefined,
    senderLocked: false,
    content: {
      to: [{ email: 'to@example.com' }],
      cc: null,
      bcc: null,
      subject: 's',
      bodyHtml: '<p>b</p>',
    },
    attachments: [],
  }
}

async function mountPane(session: ComposeSession) {
  const store = useComposeStore()
  useAccountsStore().accounts = accounts.map((a) => ({
    ...a,
    oauthClientId: '',
    signatureHtml: '',
    notifyEnabled: true,
    status: 'active',
    sortOrder: 0,
  })) as ReturnType<typeof useAccountsStore>['accounts']
  store.session = session
  store.content = {
    to: [{ email: 'to@example.com' }],
    cc: [],
    bcc: [],
    subject: 's',
    bodyHtml: '<p>b</p>',
  }
  const w = mount(ComposePane, { attachTo: document.body })
  await flushPromises()
  return { w, store }
}

describe('ComposePane', () => {
  test('new で送信元が未選択なら送信ボタンが無効（アーカイブ送信は出ない）', async () => {
    const { w } = await mountPane(newSession())
    const send = w.findAll('button').find((b) => b.text() === '送信')
    expect(send).toBeDefined()
    expect(send?.attributes('disabled')).toBeDefined()
    expect(w.text()).not.toContain('送信してアーカイブ')
    expect(w.text()).toContain('送信元を選択してください')
    w.unmount()
  })

  test('送信元ありなら送信ボタンが有効。返信では「送信してアーカイブ」も出る。ルートはドロップ先', async () => {
    const { w } = await mountPane(newSession('a1'))
    const send = w.findAll('button').find((b) => b.text() === '送信')
    expect(send?.attributes('disabled')).toBeUndefined()
    expect(w.attributes('data-file-drop-target')).toBeDefined()
    w.unmount()

    const reply = await mountPane({ ...newSession('a1'), mode: 'reply', senderLocked: true })
    expect(reply.w.text()).toContain('送信してアーカイブ')
    expect(reply.w.find('[role="combobox"][aria-label="送信元アカウント"]').exists()).toBe(false)
    expect(reply.w.text()).toContain('AI で下書き')
    reply.w.unmount()
  })

  test('新規作成には「AI で下書き」を出さない', async () => {
    const { w } = await mountPane(newSession('a1'))
    expect(w.text()).not.toContain('AI で下書き')
    w.unmount()
  })
})

function preview(over: Partial<SendPreview> = {}): SendPreview {
  return {
    confirmToken: 't',
    expiresAt: 0,
    from: { name: '仕事', email: 'work@example.com', accountId: 'a1', color: 'blue' },
    to: [{ name: '山田', email: 'yamada@example.com' }],
    cc: [{ email: 'cc@example.com' }],
    bcc: null,
    subject: '会議の件',
    attachmentCount: 2,
    bodyPreview: '明日の会議ですが',
    warnings: null,
    ...over,
  }
}

describe('SendConfirmDialog', () => {
  async function open(p: SendPreview, archive = false) {
    const store = useComposeStore()
    store.preview = p
    store.previewArchive = archive
    const w = mount(SendConfirmDialog, { attachTo: document.body })
    await flushPromises()
    return { w, store }
  }

  test('確定ボタンは「<アドレス> から送信」。送信元・宛先・件名・添付数・本文冒頭を表示する', async () => {
    const { w } = await open(preview())
    const text = document.body.textContent ?? ''
    expect(text).toContain('work@example.com から送信')
    expect(text).toContain('山田 <yamada@example.com>')
    expect(text).toContain('cc@example.com')
    expect(text).toContain('会議の件')
    expect(text).toContain('2 件')
    expect(text).toContain('明日の会議ですが')
    expect(text).toContain('キャンセル')
    expect(text).not.toContain('今後表示しない')
    w.unmount()
  })

  test('アーカイブ付きの送信は確定ボタンにそれを示す', async () => {
    const { w } = await open(preview(), true)
    expect(document.body.textContent).toContain('work@example.com から送信してアーカイブ')
    w.unmount()
  })

  test('他の自アカウント宛の警告を強調表示する', async () => {
    const { w } = await open(
      preview({
        warnings: [
          {
            code: 'OTHER_OWN_ACCOUNT',
            message: '他の自分のアカウント宛です',
            emails: ['me@example.com'],
          },
          { code: 'EMPTY_SUBJECT', message: '件名がありません' },
        ],
      }),
    )
    const items = Array.from(document.body.querySelectorAll('.warning'))
    expect(items).toHaveLength(2)
    expect(items[0].className).toContain('warning-strong')
    expect(items[0].textContent).toContain('me@example.com')
    expect(items[1].className).not.toContain('warning-strong')
    w.unmount()
  })

  test('キャンセルで preview を破棄する（エディタへ戻る）', async () => {
    const { w, store } = await open(preview())
    const cancel = Array.from(document.body.querySelectorAll('button')).find((b) =>
      b.textContent?.includes('キャンセル'),
    )
    cancel?.click()
    await flushPromises()
    expect(store.preview).toBeUndefined()
    w.unmount()
  })
})
