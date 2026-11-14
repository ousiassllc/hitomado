import { afterAll, beforeEach, describe, expect, test } from 'bun:test'
import { mount } from '@vue/test-utils'
import type { Account } from '../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import AccountRow from '../../src/components/settings/accounts/AccountRow.vue'
import { deriveBanners } from '../../src/lib/banners'
import { isTokenRevokedMessage } from '../../src/lib/tokenRevoked'
import { installMocks, resetKit, restoreMocks } from '../inboxKit'

installMocks()
afterAll(restoreMocks)
beforeEach(resetKit)

// Go（sync の MsgTokenRevoked）が載せる、Google がトークンを無効にした（invalid_grant）ときの状態メッセージ。
const revoked = '再接続が必要です（Google がトークンを無効にしました）'

const account = {
  id: 'a1',
  displayName: '仕事',
  email: 'a1@example.com',
  color: 'blue',
  notifyEnabled: true,
  status: 'needs_reauth',
  statusMessage: revoked,
} as Account

const mountRow = (statusMessage: string | undefined, status = 'needs_reauth') =>
  mount(AccountRow, {
    props: { account, index: 0, total: 1, status, statusMessage, busy: false },
  })

describe('トークンの失効（invalid_grant）の要再接続', () => {
  test('案内文の判定', () => {
    expect(isTokenRevokedMessage(revoked)).toBe(true)
    expect(isTokenRevokedMessage('再接続が必要です')).toBe(false)
    expect(isTokenRevokedMessage(undefined)).toBe(false)
  })

  test('バナーは原因（「テスト」の 7 日）を添え、操作は再接続のまま', () => {
    const reconnected: string[] = []
    const r = deriveBanners({
      online: true,
      accounts: [{ id: 'a1', displayName: '仕事' }],
      statuses: { a1: 'needs_reauth' },
      statusMessages: { a1: revoked },
      extra: [],
      reconnect: (id) => reconnected.push(id),
      openOAuthClients: () => {},
    })
    expect(r).toHaveLength(1)
    expect(r[0].message).toContain('仕事')
    expect(r[0].message).toContain('「テスト」')
    expect(r[0].message).toContain('7 日')
    expect(r[0].action?.label).toBe('再接続')
    r[0].action?.run()
    expect(reconnected).toEqual(['a1'])
  })

  test('設定のアカウント行は、固定の状態表示のまま、原因の案内を下に出す', () => {
    const w = mountRow(revoked)
    expect(w.get('.status').text()).toBe('再接続が必要です')
    const hint = w.get('.hint').text()
    expect(hint).toContain('公開ステータス')
    expect(hint).toContain('「テスト」')
    expect(hint).toContain('7 日')
    expect(w.findAll('button').map((b) => b.text())).toContain('再接続')
  })

  test('それ以外の要再接続・要再接続でない行には案内を出さない', () => {
    expect(mountRow('再接続が必要です').find('.hint').exists()).toBe(false)
    expect(mountRow(revoked, 'active').find('.hint').exists()).toBe(false)
  })
})
