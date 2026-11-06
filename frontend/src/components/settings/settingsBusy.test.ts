// 処理中の固定（O1）と進捗の置き場所（O12）の回帰テスト（#238）。
import { describe, expect, test } from 'bun:test'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { useAccountsStore } from '../../stores/accounts'
import { useSettingsStore } from '../../stores/settings'
import AccountsSettings from './accounts/AccountsSettings.vue'
import McpSettings from './McpSettings.vue'

function deferred() {
  let resolve: () => void = () => {}
  const promise = new Promise<void>((r) => {
    resolve = r
  })
  return { promise, resolve }
}

describe('AccountsSettings の削除確認（O1）', () => {
  test('削除の実行中は「キャンセル」を押せず、終わると閉じる', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const accounts = useAccountsStore()
    accounts.accounts = [
      {
        id: 'a1',
        oauthClientId: 'c1',
        email: 'a@example.com',
        displayName: '仕事',
        color: 'blue',
        signatureHtml: '',
        notifyEnabled: true,
        status: 'active',
        sortOrder: 0,
      },
    ]
    const gate = deferred()
    accounts.remove = async () => gate.promise
    const w = mount(AccountsSettings, { global: { plugins: [pinia] }, attachTo: document.body })

    const dialogButton = (label: string) =>
      [...(document.body.querySelectorAll<HTMLButtonElement>('[role="dialog"] button') ?? [])].find(
        (b) => b.textContent?.trim() === label,
      )

    await w
      .findAll('button')
      .find((b) => b.text() === '仕事 を削除')
      ?.trigger('click')
    await flushPromises()
    expect(dialogButton('キャンセル')?.disabled).toBe(false)

    dialogButton('削除する')?.click()
    await flushPromises()
    expect(dialogButton('キャンセル')?.disabled).toBe(true)

    gate.resolve()
    await flushPromises()
    w.unmount()
  })
})

describe('McpSettings の進捗（O12）', () => {
  const status = {
    enabled: true,
    host: '127.0.0.1',
    port: 47650,
    allowNonLoopback: false,
    hasToken: true,
    listening: true,
    endpoint: 'http://127.0.0.1:47650/mcp',
  }

  test('走っている操作のボタンだけにスピナーを出し、ほかは無効にするだけにする', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const store = useSettingsStore()
    store.loadMcp = async () => {}
    store.mcp = status
    const gate = deferred()
    store.issueMcpToken = async () => {
      await gate.promise
      return 'hmcp_x'
    }
    const w = mount(McpSettings, { global: { plugins: [pinia] } })
    await w.vm.$nextTick()
    const button = (label: string) => w.findAll('button').find((b) => b.text() === label)

    await button('トークンを再発行')?.trigger('click')
    const busy = w.findAll('button[aria-busy="true"]')
    expect(busy.map((b) => b.text())).toEqual(['トークンを再発行'])
    expect(button('設定を反映')?.attributes('disabled')).toBeDefined()
    expect(button('トークンを無効にする')?.attributes('disabled')).toBeDefined()

    gate.resolve()
    await flushPromises()
    expect(w.findAll('button[aria-busy="true"]')).toHaveLength(0)
    expect(button('設定を反映')?.attributes('disabled')).toBeUndefined()
    w.unmount()
  })
})
