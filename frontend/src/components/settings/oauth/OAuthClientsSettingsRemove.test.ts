import { describe, expect, test } from 'bun:test'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { useAccountsStore } from '../../../stores/accounts'
import OAuthClientsSettings from './OAuthClientsSettings.vue'

describe('OAuthClientsSettings の削除', () => {
  test('確認モーダル（対象の名前つき）を経由し、確認すると削除する', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const accounts = useAccountsStore()
    accounts.oauthClients = [
      { id: 'c2', name: '予備', clientId: 'id-2', accountCount: 0, hasSecret: true },
    ]
    const removed: string[] = []
    accounts.deleteOAuthClient = async (id: string) => {
      removed.push(id)
    }
    const w = mount(OAuthClientsSettings, { global: { plugins: [pinia] } })

    await w.get('[data-client-id="c2"] button.remove').trigger('click')
    await flushPromises()
    expect(removed).toEqual([]) // 押しただけでは消さない
    const dialog = document.body.querySelector('[role="dialog"]')
    expect(dialog?.textContent).toContain('予備 を削除しますか？')

    const confirm = [...(dialog?.querySelectorAll<HTMLButtonElement>('button') ?? [])].find(
      (b) => b.textContent?.trim() === '削除する',
    )
    expect(confirm).toBeDefined()
    confirm?.click()
    await flushPromises()
    expect(removed).toEqual(['c2'])
  })
})
