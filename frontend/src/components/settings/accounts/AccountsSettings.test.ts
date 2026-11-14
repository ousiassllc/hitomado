// 設定 → アカウントの操作（並べ替え・色・新着通知・表示名）が、store 経由でバインディングを正しく呼ぶことの回帰テスト（#355）。
import { afterAll, beforeEach, describe, expect, mock, test } from 'bun:test'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import * as realAccount from '../../../../bindings/github.com/ousiassllc/hitomado/internal/app/accountservice'
import type { Account } from '../../../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'

const BINDINGS = '../../../../bindings/github.com/ousiassllc/hitomado/internal/app/accountservice'

const state = {
  updateCalls: [] as unknown[][],
  reorderCalls: [] as string[][],
  reorderError: undefined as unknown,
  /** サーバー側の現在の並び（ReorderAccounts の戻りに使う）。 */
  server: [] as Account[],
}

mock.module(BINDINGS, () => ({
  ...realAccount,
  UpdateAccount: async (id: string, patch: unknown) => {
    state.updateCalls.push([id, patch])
    const a = state.server.find((x) => x.id === id) as Account
    return { ...a, ...(patch as object) }
  },
  ReorderAccounts: async (ids: string[]) => {
    state.reorderCalls.push(ids)
    if (state.reorderError) throw state.reorderError
    return ids.map((id) => state.server.find((x) => x.id === id) as Account)
  },
}))
afterAll(() => {
  mock.module(BINDINGS, () => ({ ...realAccount }))
})

// mock.module をストア（バインディングの静的 import）より先に効かせるため、動的に読む（test/accountsStatus.test.ts と同じ）。
const { useAccountsStore } = await import('../../../stores/accounts')
const { useUiStore } = await import('../../../stores/ui')
const { default: AccountsSettings } = await import('./AccountsSettings.vue')

const account = (id: string, displayName: string, sortOrder: number): Account =>
  ({
    id,
    oauthClientId: 'c1',
    email: `${id}@example.com`,
    displayName,
    color: 'blue',
    signatureHtml: '',
    notifyEnabled: true,
    status: 'active',
    sortOrder,
  }) as Account

function setup() {
  const pinia = createPinia()
  setActivePinia(pinia)
  const accounts = useAccountsStore()
  const ui = useUiStore()
  state.server = [account('a1', '仕事', 0), account('a2', '私用', 1), account('a3', '副業', 2)]
  accounts.accounts = [...state.server]
  const w = mount(AccountsSettings, { global: { plugins: [pinia] }, attachTo: document.body })
  return { w, accounts, ui }
}

const moveButton = (id: string, dir: 'up' | 'down') =>
  document.body.querySelector<HTMLButtonElement>(`[data-move="${id}:${dir}"]`) as HTMLButtonElement
const order = (accounts: { accounts: Account[] }) => accounts.accounts.map((a) => a.id)

beforeEach(() => {
  state.updateCalls = []
  state.reorderCalls = []
  state.reorderError = undefined
  document.body.innerHTML = ''
})

describe('並べ替え', () => {
  test('下へ移動すると、全アカウント ID を並べ替えた配列で ReorderAccounts を呼ぶ', async () => {
    const { w, accounts } = setup()
    moveButton('a1', 'down').click()
    await flushPromises()
    expect(state.reorderCalls).toEqual([['a2', 'a1', 'a3']])
    expect(order(accounts)).toEqual(['a2', 'a1', 'a3'])
    w.unmount()
  })

  test('上へ移動すると、1 つ前へ入れ替えた配列で呼ぶ', async () => {
    const { w } = setup()
    moveButton('a3', 'up').click()
    await flushPromises()
    expect(state.reorderCalls).toEqual([['a1', 'a3', 'a2']])
    w.unmount()
  })

  test('先頭の「上へ」・末尾の「下へ」は押せず、呼ばない', async () => {
    const { w } = setup()
    expect(moveButton('a1', 'up').disabled).toBe(true)
    expect(moveButton('a3', 'down').disabled).toBe(true)
    // 無効のボタンは click() でも発火しない。move 側の範囲チェックと合わせて、呼ばれないことを確かめる
    moveButton('a1', 'up').click()
    moveButton('a3', 'down').click()
    await flushPromises()
    expect(state.reorderCalls).toEqual([])
    w.unmount()
  })

  test('移動後は移動したアカウントのボタンにフォーカスが戻り、「N 番目」を読み上げる', async () => {
    const { w } = setup()
    moveButton('a1', 'down').click()
    await flushPromises()
    expect(document.activeElement).toBe(moveButton('a1', 'down'))
    expect(w.get('p[role="status"]').text()).toBe('仕事 を 3 件中 2 番目へ移動しました。')
    w.unmount()
  })

  test('端に着いて押せなくなったときは、反対向きのボタンにフォーカスを渡す', async () => {
    const { w } = setup()
    moveButton('a2', 'down').click()
    await flushPromises()
    // a2 は末尾になり「下へ」が無効なので「上へ」へ
    expect(moveButton('a2', 'down').disabled).toBe(true)
    expect(document.activeElement).toBe(moveButton('a2', 'up'))
    expect(w.get('p[role="status"]').text()).toBe('私用 を 3 件中 3 番目へ移動しました。')
    w.unmount()
  })

  test('失敗したらエラーアラートを出し、並びも読み上げも変えない', async () => {
    const { w, accounts, ui } = setup()
    state.reorderError = {
      cause: JSON.stringify({
        code: 'INTERNAL',
        message: '並べ替えに失敗しました。',
        retryable: false,
      }),
    }
    moveButton('a1', 'down').click()
    await flushPromises()
    expect(state.reorderCalls).toEqual([['a2', 'a1', 'a3']])
    expect(ui.alerts.map((a) => [a.kind, a.message])).toEqual([
      ['error', '並べ替えに失敗しました。'],
    ])
    expect(order(accounts)).toEqual(['a1', 'a2', 'a3'])
    expect(w.get('p[role="status"]').text()).toBe('')
    w.unmount()
  })
})

describe('アカウント色', () => {
  test('色を選ぶと UpdateAccount(id, { color }) を呼ぶ', async () => {
    const { w, accounts } = setup()
    const radio = w.get('li[data-account-id="a2"] input[type="radio"][aria-label="色: 赤"]')
    await radio.setValue(true)
    await flushPromises()
    expect(state.updateCalls).toEqual([['a2', { color: 'red' }]])
    expect(accounts.accounts.find((a) => a.id === 'a2')?.color).toBe('red')
    w.unmount()
  })
})

describe('新着通知', () => {
  test('スイッチを切り替えると UpdateAccount(id, { notifyEnabled }) を呼ぶ', async () => {
    const { w, accounts } = setup()
    await w.get('button[role="switch"][aria-label="私用 の新着通知"]').trigger('click')
    await flushPromises()
    expect(state.updateCalls).toEqual([['a2', { notifyEnabled: false }]])
    expect(accounts.accounts.find((a) => a.id === 'a2')?.notifyEnabled).toBe(false)

    await w.get('button[role="switch"][aria-label="私用 の新着通知"]').trigger('click')
    await flushPromises()
    expect(state.updateCalls[1]).toEqual(['a2', { notifyEnabled: true }])
    w.unmount()
  })
})

describe('表示名', () => {
  const saveButton = (w: VueWrapper) =>
    w
      .get('li[data-account-id="a1"]')
      .findAll('button')
      .find((b) => b.text() === '保存')

  test('変更して保存すると、前後の空白を除いて UpdateAccount(id, { displayName }) を呼ぶ', async () => {
    const { w, accounts } = setup()
    const input = w.get('li[data-account-id="a1"] input[type="text"]')
    await input.setValue('  会社  ')
    await saveButton(w)?.trigger('click')
    await flushPromises()
    expect(state.updateCalls).toEqual([['a1', { displayName: '会社' }]])
    expect(accounts.accounts.find((a) => a.id === 'a1')?.displayName).toBe('会社')
    w.unmount()
  })

  test('変更が無いときは保存できず、呼ばない', async () => {
    const { w } = setup()
    expect(saveButton(w)?.attributes('disabled')).toBeDefined()
    await w.get('li[data-account-id="a1"] input[type="text"]').trigger('keydown', { key: 'Enter' })
    await flushPromises()
    expect(state.updateCalls).toEqual([])
    w.unmount()
  })

  test('空欄は displayNameRequired を出して保存できない（ボタンも Enter も）', async () => {
    const { w } = setup()
    const input = w.get('li[data-account-id="a1"] input[type="text"]')
    await input.setValue('   ')
    expect(w.get('li[data-account-id="a1"]').text()).toContain('表示名を入力してください。')
    expect(saveButton(w)?.attributes('disabled')).toBeDefined()
    await input.trigger('keydown', { key: 'Enter' })
    await flushPromises()
    expect(state.updateCalls).toEqual([])
    w.unmount()
  })
})
