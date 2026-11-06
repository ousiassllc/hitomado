import { beforeEach, describe, expect, test } from 'bun:test'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { defineComponent } from 'vue'
import type { Account, SendPreview } from '../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import { useAccountsStore } from '../stores/accounts'
import { useComposeStore } from '../stores/compose'
import { useSettingsStore } from '../stores/settings'
import { useUiStore } from '../stores/ui'
import SendConfirmDialog from './compose/SendConfirmDialog.vue'
import AccountsSettings from './settings/accounts/AccountsSettings.vue'
import { acctA } from './settings/ai/aiTestSupport'
import ChatGPTPlanNotice from './settings/ai/ChatGPTPlanNotice.vue'
import AlertStack from './shell/AlertStack.vue'

// 退出のフェード（ガイド M5）の間は、ダイアログの中身を変えない。
// happy-dom は閉じた瞬間に中身を外すので、BaseDialog を「title・description・本文を常に出すスタブ」に差し替え、
// 閉じたあとにも文言が残っているかを確かめる。
const DialogStub = defineComponent({
  props: { open: Boolean, title: { type: String, default: '' }, description: { type: String, default: '' } },
  template: '<div class="dlg" :data-open="String(open)"><h2>{{ title }}</h2><p>{{ description }}</p><slot /><slot name="footer" /></div>',
})
const stubs = { BaseDialog: DialogStub }

beforeEach(() => {
  document.body.innerHTML = ''
  setActivePinia(createPinia())
})

const account: Account = {
  id: 'a1',
  oauthClientId: 'c1',
  email: 'work@example.com',
  displayName: '仕事',
  color: 'blue',
  signatureHtml: '',
  notifyEnabled: true,
  status: 'active',
  sortOrder: 0,
}

describe('AccountsSettings の削除確認', () => {
  test('削除が済んで閉じても、退出の間は名前とアドレスの入った文言のまま', async () => {
    const accounts = useAccountsStore()
    accounts.accounts = [account]
    accounts.remove = async () => {}
    const w = mount(AccountsSettings, { global: { stubs } })
    const dlg = () => w.get('.dlg')

    const removeBtn = w.findAll('button').find((b) => b.text().includes('仕事') && b.text().includes('削除'))
    await removeBtn?.trigger('click')
    expect(dlg().attributes('data-open')).toBe('true')
    expect(dlg().text()).toContain('仕事 を削除しますか？')

    const confirm = w.findAll('button').find((b) => b.text().trim() === '削除する')
    await confirm?.trigger('click')
    await flushPromises()
    expect(dlg().attributes('data-open')).toBe('false')
    expect(dlg().text()).toContain('仕事 を削除しますか？')
    expect(dlg().text()).toContain('work@example.com')
    w.unmount()
  })
})

describe('ChatGPTPlanNotice', () => {
  test('確認済みで閉じても、退出の間はアカウントのメールアドレスの行を残す', async () => {
    useSettingsStore().ackChatGPTPlanNotice = async () => {}
    useUiStore()
    const w = mount(ChatGPTPlanNotice, {
      props: { accounts: [{ ...acctA, planNoticePending: true }] },
      global: { stubs },
    })
    expect(w.get('.dlg').attributes('data-open')).toBe('true')
    expect(w.text()).toContain('a@chatgpt.example')

    await w.get('[data-testid="ai-chatgpt-notice-gotit"]').trigger('click')
    await flushPromises()
    expect(w.get('.dlg').attributes('data-open')).toBe('false')
    expect(w.text()).toContain('a@chatgpt.example')
    w.unmount()
  })
})

describe('SendConfirmDialog', () => {
  const preview: SendPreview = {
    confirmToken: 't',
    expiresAt: 0,
    from: { name: '仕事', email: 'work@example.com', accountId: 'a1', color: 'blue' },
    to: [{ name: '山田', email: 'yamada@example.com' }],
    cc: null,
    bcc: null,
    subject: '会議の件',
    attachmentCount: 0,
    bodyPreview: '明日の会議ですが',
    warnings: null,
  }

  test('キャンセルで閉じても、退出の間は宛先・件名・確定ボタンの文言を残す', async () => {
    const compose = useComposeStore()
    compose.previewArchive = true
    compose.preview = preview
    const w = mount(SendConfirmDialog, { global: { stubs } })
    expect(w.get('.dlg').attributes('data-open')).toBe('true')

    compose.preview = undefined
    compose.previewArchive = false
    await flushPromises()
    expect(w.get('.dlg').attributes('data-open')).toBe('false')
    expect(w.text()).toContain('会議の件')
    expect(w.text()).toContain('山田 <yamada@example.com>')
    expect(w.text()).toContain('work@example.com から送信してアーカイブ')
    w.unmount()
  })
})

describe('AlertStack', () => {
  test('0 件でも TransitionGroup のコンテナを描画し続ける（最初の出現・最後の消去にフェードを付けるため）', async () => {
    const ui = useUiStore()
    const w = mount(AlertStack, { global: { stubs: { TransitionGroup: false } } })
    expect(w.find('[data-testid="alerts"]').exists()).toBe(true)
    expect(w.findAll('.alert')).toHaveLength(0)

    const id = ui.pushAlert('失敗しました')
    await flushPromises()
    expect(w.findAll('.alert')).toHaveLength(1)

    ui.dismissAlert(id)
    await flushPromises()
    expect(w.find('[data-testid="alerts"]').exists()).toBe(true)
    w.unmount()
  })
})
