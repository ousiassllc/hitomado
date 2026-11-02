import { describe, expect, test } from 'bun:test'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { nextTick } from 'vue'
import type { ThreadSummary } from '../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import AccountChip from '../src/components/account/AccountChip.vue'
import ThreadRow from '../src/components/inbox/ThreadRow.vue'
import BaseButton from '../src/components/ui/BaseButton.vue'
import BaseDialog from '../src/components/ui/BaseDialog.vue'
import BaseIconButton from '../src/components/ui/BaseIconButton.vue'
import { formatDate } from '../src/lib/format'

setActivePinia(createPinia())

const thread: ThreadSummary = {
  accountId: 'a1',
  threadId: 't1',
  subject: '見積のご確認',
  snippet: 'お世話になっております',
  lastMessageAt: Date.now(),
  lastFrom: { name: '山田太郎', email: 'yamada@example.com' },
  messageCount: 3,
  isUnread: true,
  hasAttachment: true,
  inInbox: true,
  labels: [{ id: 'L1', name: '重要', colorBg: '#ff0000', colorFg: '#ffffff' }],
}
const account = { displayName: '会社A', email: 'a@example.com', color: 'teal', status: 'active' }

describe('ThreadRow', () => {
  test('差出人・件名・スニペット・日時・アカウント名・添付・ラベルを表示する', () => {
    const w = mount(ThreadRow, { props: { thread, selected: false, account } })
    const text = w.text()
    expect(text).toContain('山田太郎')
    expect(text).toContain('見積のご確認')
    expect(text).toContain('お世話になっております')
    expect(text).toContain('会社A')
    expect(text).toContain('重要')
    expect(text).toContain(formatDate(thread.lastMessageAt))
    expect(w.find('[aria-label="添付ファイルあり"]').exists()).toBe(true)
    expect(w.classes()).toContain('unread')
    expect(w.find('.band').attributes('style')).toContain('var(--account-teal)')
  })

  test('既読・添付なし・差出人名なしの表示、選択状態', () => {
    const w = mount(ThreadRow, {
      props: {
        thread: { ...thread, isUnread: false, hasAttachment: false, lastFrom: { email: 'x@example.com' }, subject: '' },
        selected: true,
        account,
      },
    })
    expect(w.classes()).not.toContain('unread')
    expect(w.classes()).toContain('selected')
    expect(w.attributes('aria-selected')).toBe('true')
    expect(w.text()).toContain('x@example.com')
    expect(w.text()).toContain('(件名なし)')
    expect(w.find('[aria-label="添付ファイルあり"]').exists()).toBe(false)
  })

  test('needs_reauth のアカウントの行は淡色', () => {
    const w = mount(ThreadRow, { props: { thread, selected: false, account: { ...account, status: 'needs_reauth' } } })
    expect(w.classes()).toContain('faded')
  })
})

describe('AccountChip', () => {
  test('色だけに頼らず名前（任意でアドレス）を併記する', () => {
    const w = mount(AccountChip, { props: { account: { displayName: '会社A', email: 'a@example.com', color: 'red' }, showEmail: true } })
    expect(w.text()).toContain('会社A')
    expect(w.text()).toContain('a@example.com')
    expect(w.attributes('style')).toContain('var(--account-red)')
    expect(w.attributes('style')).toContain('var(--account-red-text)')
  })
})

describe('BaseButton / BaseIconButton', () => {
  test('loading 中は無効化されスピナーを出す', () => {
    const w = mount(BaseButton, { props: { loading: true }, slots: { default: '送信' } })
    expect(w.attributes('disabled')).toBeDefined()
    expect(w.find('[role="status"]').exists()).toBe(true)
  })

  test('アイコンボタンは aria-label を持つ', () => {
    const w = mount(BaseIconButton, { props: { label: '更新' }, slots: { default: '⟳' } })
    expect(w.attributes('aria-label')).toBe('更新')
  })
})

describe('BaseDialog', () => {
  const opts = { attachTo: document.body, global: { stubs: {} } }

  test('open=true でタイトル・本文・フッターを表示し、閉じるで update:open(false)', async () => {
    const w = mount(BaseDialog, {
      ...opts,
      props: { open: true, title: '設定', description: '説明文' },
      slots: { default: '<p id="dlg-body">本文</p>', footer: '<button id="dlg-foot">OK</button>' },
    })
    await nextTick()
    expect(document.body.textContent).toContain('設定')
    expect(document.body.textContent).toContain('説明文')
    expect(document.getElementById('dlg-body')).not.toBeNull()
    expect(document.getElementById('dlg-foot')).not.toBeNull()
    expect(document.querySelector('[role="dialog"]')).not.toBeNull()
    document.querySelector<HTMLElement>('button[aria-label="閉じる"]')?.click()
    await nextTick()
    expect(w.emitted('update:open')?.[0]).toEqual([false])
    w.unmount()
  })

  test('open=false では何も表示しない', async () => {
    const w = mount(BaseDialog, { ...opts, props: { open: false, title: '非表示' } })
    await nextTick()
    expect(document.querySelector('[role="dialog"]')).toBeNull()
    w.unmount()
  })

  test('closable=false は閉じるボタンが無く、閉じる要求も通さない', async () => {
    const w = mount(BaseDialog, { ...opts, props: { open: true, title: '確認', closable: false } })
    await nextTick()
    expect(document.querySelector('button[aria-label="閉じる"]')).toBeNull()
    w.unmount()
  })
})
