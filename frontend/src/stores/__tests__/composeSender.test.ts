import { afterAll, beforeEach, describe, expect, test } from 'bun:test'
import { buildSignatureBlock, findSignature } from '../../lib/signature'
import {
  appError,
  count,
  h,
  installMocks,
  openReply,
  reset,
  restoreMocks,
  SIG_A,
  session,
  setup,
  type useComposeStore,
} from './composeHarness'

installMocks()
afterAll(restoreMocks)
beforeEach(reset)

describe('setSender と署名', () => {
  async function openNew(compose: ReturnType<typeof useComposeStore>, accountId?: string) {
    const body = accountId ? `<p></p>${buildSignatureBlock(accountId, SIG_A)}` : '<p></p>'
    h.responses.OpenCompose = session({
      mode: 'new',
      senderLocked: false,
      accountId,
      from: accountId ? { email: 'x@example.com' } : undefined,
      content: { to: null, cc: null, bcc: null, subject: '', bodyHtml: body },
    })
    await compose.open({ mode: 'new', accountId })
  }

  test('返信・転送では送信元を変更できない', async () => {
    const { compose } = setup()
    await openReply(compose)
    expect(await compose.setSender('a2')).toBe(false)
    expect(count('SetComposeSender')).toBe(0)
  })

  test('未選択から選ぶと、そのアカウントの署名を入れる。署名だけでは dirty にならない', async () => {
    const { compose } = setup()
    await openNew(compose)
    expect(findSignature(compose.content.bodyHtml)).toBeUndefined()
    expect(await compose.setSender('a2')).toBe(true)
    expect(compose.session?.accountId).toBe('a2')
    expect(findSignature(compose.content.bodyHtml)?.accountId).toBe('a2')
    expect(compose.content.bodyHtml).toContain('Bの署名')
    expect(compose.dirty).toBe(false)
    expect(count('SaveDraft')).toBe(0)
  })

  test('未編集の署名は切替先の署名に差し替わる。署名なしアカウントへはブロックを削除', async () => {
    const { compose } = setup()
    await openNew(compose, 'a1')
    await compose.setSender('a2')
    expect(compose.content.bodyHtml).toContain('Bの署名')
    expect(compose.content.bodyHtml).not.toContain('Aの署名')
    expect(compose.signatureConflict).toBeUndefined()
    await compose.setSender('a3')
    expect(findSignature(compose.content.bodyHtml)).toBeUndefined()
    await compose.setSender('a1')
    expect(compose.content.bodyHtml).toContain('Aの署名')
  })

  test('編集済みの署名は差し替えず確認待ちにし、結果に従う', async () => {
    const { compose } = setup()
    await openNew(compose, 'a1')
    compose.content.bodyHtml = compose.content.bodyHtml.replace('Aの署名', 'Aの署名（編集）')
    await compose.setSender('a2')
    expect(compose.signatureConflict).toEqual({ accountId: 'a2' })
    expect(compose.content.bodyHtml).toContain('編集')
    compose.resolveSignatureConflict(false)
    expect(compose.signatureConflict).toBeUndefined()
    expect(compose.content.bodyHtml).toContain('編集')

    await compose.setSender('a3')
    expect(compose.signatureConflict).toEqual({ accountId: 'a3' })
    compose.resolveSignatureConflict(true)
    expect(findSignature(compose.content.bodyHtml)).toBeUndefined()
  })

  test('送信元を変えると確認済みの preview は破棄される', async () => {
    const { compose } = setup()
    await openNew(compose, 'a1')
    compose.content.to = [{ email: 'to@example.com' }]
    await compose.prepareSend(false)
    expect(compose.preview).toBeDefined()
    await compose.setSender('a2')
    expect(compose.preview).toBeUndefined()
  })

  test('SetComposeSender が失敗したら署名・送信元を変えない', async () => {
    const { compose } = setup()
    await openNew(compose, 'a1')
    h.failures.SetComposeSender = appError('SESSION_NOT_FOUND')
    expect(await compose.setSender('a2')).toBe(false)
    expect(compose.session?.accountId).toBe('a1')
    expect(compose.content.bodyHtml).toContain('Aの署名')
    expect(compose.error).toBeDefined()
  })
})
