import { beforeEach, describe, expect, test } from 'bun:test'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { common } from '../../lib/messages'
import { useAccountsStore } from '../../stores/accounts'
import { useUiStore } from '../../stores/ui'
import AddAccountDialog from './AddAccountDialog.vue'

// 認可待ちステップは本文にフォーカス可能な要素が無いので、本文を名前付きの region にする（ガイド B6）。
beforeEach(() => {
  document.body.innerHTML = ''
  setActivePinia(createPinia())
})

const body = () => document.body.querySelector('.dlg-body')

describe('AddAccountDialog の本文領域', () => {
  test('認可待ちの間だけ本文が tabindex=0・role=region・aria-label を持つ', async () => {
    const accounts = useAccountsStore()
    accounts.oauthClients = [{ id: 'c1', name: 'Client' }] as typeof accounts.oauthClients
    accounts.loadOAuthClients = async () => {}
    accounts.add = () => new Promise(() => {}) // 認可が完了しない状態を保つ
    const ui = useUiStore()
    const w = mount(AddAccountDialog, { attachTo: document.body })
    ui.openDialog('addAccount')
    await flushPromises()

    expect(body()?.hasAttribute('tabindex')).toBe(false)

    const start = [...document.body.querySelectorAll('button')].find(
      (b) => b.textContent?.trim() === common.addAccount.start,
    )
    start?.click()
    await flushPromises()

    expect(body()?.getAttribute('tabindex')).toBe('0')
    expect(body()?.getAttribute('role')).toBe('region')
    expect(body()?.getAttribute('aria-label')).toBe(common.addAccount.waiting)
    w.unmount()
  })
})
