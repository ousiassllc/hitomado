import { afterAll, beforeEach, describe, expect, test } from 'bun:test'
import type { OpenComposeRequest } from '../../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import { buildSignatureBlock, findSignature } from '../../lib/signature'
import {
  calls,
  h,
  installMocks,
  reset,
  restoreMocks,
  SIG_A,
  session,
  setup,
  useInboxStore,
} from './composeHarness'

installMocks()
afterAll(restoreMocks)
beforeEach(reset)

/** Go 側と同じく、accountId があればその署名つきで、なければ未選択で新規作成セッションを返す。 */
function respondLikeBackend(): void {
  h.responses.OpenCompose = (req: OpenComposeRequest) =>
    session({
      mode: 'new',
      senderLocked: false,
      accountId: req.accountId,
      from: req.accountId ? { email: 'a1@example.com' } : undefined,
      content: {
        to: null,
        cc: null,
        bcc: null,
        subject: '',
        bodyHtml: req.accountId ? `<p></p>${buildSignatureBlock(req.accountId, SIG_A)}` : '<p></p>',
      },
    })
}

const requested = () => calls.filter((c) => c.name === 'OpenCompose').map((c) => c.args[0])

describe('新規作成の送信元の初期値（F-11）', () => {
  test('アカウントフィルタが単一選択なら、そのアカウントを送信元に開き、署名が入る', async () => {
    const { compose } = setup()
    respondLikeBackend()
    useInboxStore().filter.accountIds = ['a1']
    expect(await compose.open({ mode: 'new' })).toBe(true)
    expect(requested()).toEqual([{ mode: 'new', accountId: 'a1' }])
    expect(compose.session?.accountId).toBe('a1')
    expect(findSignature(compose.content.bodyHtml)?.accountId).toBe('a1')
    expect(compose.canSend).toBe(true)
  })

  test.each([
    ['すべて（0 件）', []],
    ['2 件以上', ['a1', 'a2']],
  ])('フィルタが %s なら送信元は未選択で、送信できない', async (_name, ids) => {
    const { compose } = setup()
    respondLikeBackend()
    useInboxStore().filter.accountIds = ids
    expect(await compose.open({ mode: 'new' })).toBe(true)
    expect(requested()).toEqual([{ mode: 'new' }])
    expect(compose.session?.accountId).toBeUndefined()
    expect(compose.canSend).toBe(false)
  })

  test('呼び出し元が accountId を指定していればフィルタより優先する', async () => {
    const { compose } = setup()
    respondLikeBackend()
    useInboxStore().filter.accountIds = ['a1']
    await compose.open({ mode: 'new', accountId: 'a2' })
    expect(requested()).toEqual([{ mode: 'new', accountId: 'a2' }])
  })

  test('返信ではフィルタを送信元に使わない', async () => {
    const { compose } = setup()
    useInboxStore().filter.accountIds = ['a2']
    h.responses.OpenCompose = session()
    await compose.open({ mode: 'reply', ref: { accountId: 'a1', threadId: 't' } })
    expect(requested()).toEqual([{ mode: 'reply', ref: { accountId: 'a1', threadId: 't' } }])
  })
})
