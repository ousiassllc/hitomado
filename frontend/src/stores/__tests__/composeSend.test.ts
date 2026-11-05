import { afterAll, beforeEach, describe, expect, test } from 'bun:test'
import {
  appError,
  calls,
  count,
  h,
  installMocks,
  openReply,
  previewOf,
  reset,
  restoreMocks,
  setup,
  useInboxStore,
} from './composeHarness'

installMocks()
afterAll(restoreMocks)
beforeEach(reset)

describe('編集・確認の無効化', () => {
  test('編集すると dirty になり、確認済みの preview は破棄される（再確認が必要）', async () => {
    const { compose, ui } = setup()
    await openReply(compose)
    expect(await compose.prepareSend(false)).toBe(true)
    expect(compose.preview?.confirmToken).toBe('t1')
    expect(ui.dialogs.sendConfirm).toBe(true)
    compose.content.subject = 'changed'
    expect(compose.preview).toBeUndefined()
    expect(ui.dialogs.sendConfirm).toBe(false)
    expect(compose.dirty).toBe(true)
    expect(await compose.confirmSend()).toBe(false)
    expect(count('Send')).toBe(0)
  })

  test('宛先・添付・本文の変更でも破棄される', async () => {
    const { compose } = setup()
    await openReply(compose)
    for (const edit of [
      () => compose.content.to.push({ email: 'x@example.com' }),
      () => {
        compose.content.bodyHtml += '<p>x</p>'
      },
      () => {
        compose.attachments = [{ id: 'f', filename: 'a', size: 1, source: 'local' }]
      },
    ]) {
      await compose.prepareSend(false)
      expect(compose.preview).toBeDefined()
      edit()
      expect(compose.preview).toBeUndefined()
    }
  })

  test('確認の準備中に内容が変わったら preview を採用しない', async () => {
    const { compose } = setup()
    await openReply(compose)
    const gate = Promise.withResolvers<void>()
    h.gates.PrepareSend = gate.promise
    const p = compose.prepareSend(false)
    compose.content.subject = 'changed while preparing'
    gate.resolve()
    expect(await p).toBe(false)
    expect(compose.preview).toBeUndefined()
    expect(compose.error).toContain('もう一度')
  })

  test('キャンセルで preview を破棄する（セッションは維持）', async () => {
    const { compose } = setup()
    await openReply(compose)
    await compose.prepareSend(false)
    compose.cancelPreview()
    expect(compose.preview).toBeUndefined()
    expect(compose.session).toBeDefined()
  })
})

describe('prepareSend のエラー', () => {
  test('送信元未選択は呼び出さずに SENDER_REQUIRED を表示', async () => {
    const { compose } = setup()
    await openReply(compose, {
      mode: 'new',
      senderLocked: false,
      accountId: undefined,
      from: undefined,
    })
    expect(compose.canSend).toBe(false)
    expect(await compose.prepareSend(false)).toBe(false)
    expect(count('PrepareSend')).toBe(0)
    expect(compose.error).toContain('送信元')
  })

  test.each([
    ['NO_RECIPIENTS', {}, '宛先'],
    ['INVALID_ADDRESS', { details: { emails: ['bad@'] } }, 'bad@'],
    ['SENDER_MISMATCH', {}, '送信元'],
  ])('%s は固定文言のエラーを表示し preview を出さない', async (code, extra, text) => {
    const { compose } = setup()
    await openReply(compose)
    h.failures.PrepareSend = appError(code, extra)
    expect(await compose.prepareSend(false)).toBe(false)
    expect(compose.preview).toBeUndefined()
    expect(compose.error).toContain(text)
  })

  test('「送信してアーカイブ」は返信・転送のみ（新規では archiveAfterSend=false）', async () => {
    const { compose } = setup()
    await openReply(compose)
    await compose.prepareSend(true)
    expect(calls.find((c) => c.name === 'PrepareSend')?.args[2]).toEqual({ archiveAfterSend: true })
    compose.cancelPreview()
    await compose.discard()
    await openReply(compose, { mode: 'new', senderLocked: false })
    await compose.prepareSend(true)
    expect(calls.filter((c) => c.name === 'PrepareSend')[1].args[2]).toEqual({
      archiveAfterSend: false,
    })
  })
})

describe('confirmSend', () => {
  test('成功でセッションを閉じ、トーストを出し、受信箱を更新する', async () => {
    const { compose, ui } = setup()
    await openReply(compose)
    await compose.prepareSend(false)
    expect(await compose.confirmSend()).toBe(true)
    const send = calls.find((c) => c.name === 'Send')
    expect(send?.args[0]).toBe('s1')
    expect(send?.args[1]).toBe('t1')
    expect(compose.session).toBeUndefined()
    expect(compose.preview).toBeUndefined()
    expect(ui.toast).toMatchObject({ kind: 'success', message: '送信しました' })
    expect(count('ListInbox')).toBeGreaterThan(0)
  })

  test('送信してアーカイブが成功したらスレッドを一覧から外す', async () => {
    const { compose, ui } = setup()
    const inbox = useInboxStore()
    inbox.items = [{ accountId: 'a1', threadId: 't', subject: 's' } as (typeof inbox.items)[number]]
    inbox.mode = 'inbox'
    await openReply(compose)
    await compose.prepareSend(true)
    h.responses.Send = { messageId: 'm', threadId: 't', archived: true }
    expect(await compose.confirmSend()).toBe(true)
    expect(inbox.items.find((i) => i.threadId === 't')).toBeUndefined()
    expect(ui.toast?.message).toContain('アーカイブ')
  })

  test('アーカイブ失敗は送信成功として扱い、警告アラートを出す', async () => {
    const { compose, ui } = setup()
    await openReply(compose)
    await compose.prepareSend(true)
    h.responses.Send = {
      messageId: 'm',
      threadId: 't',
      archived: false,
      archiveError: { code: 'GMAIL_ERROR' },
    }
    expect(await compose.confirmSend()).toBe(true)
    expect(compose.session).toBeUndefined()
    expect(ui.alerts.map((a) => a.message).join('|')).toContain('アーカイブできませんでした')
  })

  test('失敗時はエディタを閉じず本文・添付・宛先を保持し、再度の確認から再送できる', async () => {
    const { compose } = setup()
    await openReply(compose)
    compose.attachments = [{ id: 'f', filename: 'a.txt', size: 3, source: 'local' }]
    compose.content.bodyHtml = '<p>大事な本文</p>'
    await compose.prepareSend(false)
    h.failures.Send = appError('OFFLINE', { retryable: true })
    expect(await compose.confirmSend()).toBe(false)
    expect(compose.session?.sessionId).toBe('s1')
    expect(compose.content.bodyHtml).toBe('<p>大事な本文</p>')
    expect(compose.attachments).toHaveLength(1)
    expect(compose.content.to).toEqual([{ email: 'to@example.com' }])
    expect(compose.preview).toBeUndefined()
    expect(compose.sending).toBe(false)
    expect(compose.error).toContain('送信に失敗')
    expect(compose.dirty).toBe(true)

    h.failures = {}
    h.responses.PrepareSend = previewOf('t2')
    expect(await compose.prepareSend(false)).toBe(true)
    expect(await compose.confirmSend()).toBe(true)
    expect(calls.filter((c) => c.name === 'Send')[1].args[1]).toBe('t2')
  })

  test.each([
    ['CONFIRM_EXPIRED', '有効期限'],
    ['CONFIRM_MISMATCH', '内容が変わりました'],
    ['CONFIRM_REQUIRED', '確認が必要'],
  ])('%s は再度 prepareSend へ誘導する', async (code, text) => {
    const { compose } = setup()
    await openReply(compose)
    await compose.prepareSend(false)
    h.failures.Send = appError(code)
    expect(await compose.confirmSend()).toBe(false)
    expect(compose.error).toContain(text)
    expect(compose.error).toContain('もう一度「送信」')
    expect(compose.session).toBeDefined()
    expect(compose.preview).toBeUndefined()
  })

  test('SEND_UNCERTAIN は「送信に失敗」と言わず、送信済みの確認を促す', async () => {
    const { compose } = setup()
    await openReply(compose)
    await compose.prepareSend(false)
    h.failures.Send = appError('SEND_UNCERTAIN')
    expect(await compose.confirmSend()).toBe(false)
    expect(compose.error).toContain('送信済みの可能性')
    expect(compose.error).not.toContain('送信に失敗')
    expect(compose.session).toBeDefined()
  })

  test('送信中の二重送信を防ぐ', async () => {
    const { compose } = setup()
    await openReply(compose)
    await compose.prepareSend(false)
    const gate = Promise.withResolvers<void>()
    h.gates.Send = gate.promise
    const first = compose.confirmSend()
    expect(compose.sending).toBe(true)
    expect(compose.canSend).toBe(false)
    expect(await compose.confirmSend()).toBe(false)
    expect(await compose.prepareSend(false)).toBe(false)
    expect(await compose.close()).toBe(false)
    gate.resolve()
    expect(await first).toBe(true)
    expect(count('Send')).toBe(1)
  })

  test('確認なし（preview なし）では送信しない', async () => {
    const { compose } = setup()
    await openReply(compose)
    expect(await compose.confirmSend()).toBe(false)
    expect(count('Send')).toBe(0)
  })
})
