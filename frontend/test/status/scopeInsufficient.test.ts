import { afterAll, beforeEach, describe, expect, test } from 'bun:test'
import { mount } from '@vue/test-utils'
import AccountRow from '../../src/components/settings/accounts/AccountRow.vue'
import { deriveBanners } from '../../src/lib/banners'
import { isScopeInsufficientMessage } from '../../src/lib/scopeInsufficient'
import { installMocks, resetKit, restoreMocks } from '../inboxKit'

installMocks()
afterAll(restoreMocks)
beforeEach(resetKit)

// Go（sync の msgScopeInsufficient／apperr の NEEDS_REAUTH）が載せる、権限（スコープ）不足の案内文。
const scope = 'Gmail の権限（スコープ）が足りません。再接続して、すべての権限を許可してください'

describe('権限（スコープ）不足の要再接続', () => {
  test('案内文の判定', () => {
    expect(isScopeInsufficientMessage(scope)).toBe(true)
    expect(isScopeInsufficientMessage('再接続が必要です')).toBe(false)
    expect(isScopeInsufficientMessage(undefined)).toBe(false)
  })

  test('バナーは Go のメッセージを出し、操作は再接続のまま', () => {
    const reconnected: string[] = []
    const r = deriveBanners({
      online: true,
      accounts: [{ id: 'a1', displayName: '仕事' }],
      statuses: { a1: 'needs_reauth' },
      statusMessages: { a1: scope },
      extra: [],
      reconnect: (id) => reconnected.push(id),
      openOAuthClients: () => {},
    })
    expect(r).toHaveLength(1)
    expect(r[0].message).toContain(scope)
    expect(r[0].message).not.toContain('接続が切れました')
    expect(r[0].action?.label).toBe('再接続')
    r[0].action?.run()
    expect(reconnected).toEqual(['a1'])
  })

  test('設定のアカウント行は Go のメッセージを出し、OAuth クライアントの確認ボタンは出さない', () => {
    const account = {
      id: 'a1',
      displayName: '仕事',
      email: 'a1@example.com',
      color: 'blue',
      notifyEnabled: true,
      status: 'needs_reauth',
      statusMessage: scope,
    }
    const w = mount(AccountRow, {
      props: {
        account,
        index: 0,
        total: 1,
        status: 'needs_reauth',
        statusMessage: scope,
        busy: false,
      },
    })
    expect(w.get('.status').text()).toBe(scope)
    expect(w.findAll('button').map((b) => b.text())).not.toContain('OAuth クライアントを確認')
  })
})
