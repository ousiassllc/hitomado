import { afterAll, beforeEach, describe, expect, test } from 'bun:test'
import { findSignature } from '../../lib/signature'
import {
  appError,
  count,
  h,
  installMocks,
  names,
  openReply,
  reset,
  restoreMocks,
  session,
  setup,
} from './composeHarness'

installMocks()
afterAll(restoreMocks)
beforeEach(reset)

describe('open', () => {
  test('セッションを開き、null の配列を空配列に正規化する。同じスレッドは再度開かない', async () => {
    const { compose } = setup()
    await openReply(compose)
    expect(compose.session?.sessionId).toBe('s1')
    expect(compose.content.cc).toEqual([])
    expect(compose.attachments).toEqual([])
    expect(compose.dirty).toBe(false)
    expect(compose.isOpenForThread({ accountId: 'a1', threadId: 't' })).toBe(true)
    expect(compose.isOpenForThread({ accountId: 'a1', threadId: 'x' })).toBe(false)
    expect(compose.isNew).toBe(false)
    expect(compose.canArchive).toBe(true)
    expect(await compose.open({ mode: 'reply', ref: { accountId: 'a1', threadId: 't' } })).toBe(
      true,
    )
    expect(count('OpenCompose')).toBe(1)
  })

  test('開けなければエラートーストを出して false', async () => {
    const { compose, ui } = setup()
    h.failures.OpenCompose = appError('NOT_FOUND')
    expect(await compose.open({ mode: 'new' })).toBe(false)
    expect(compose.session).toBeUndefined()
    expect(ui.toasts.at(-1)?.kind).toBe('error')
  })

  test('別のセッションが開いていれば保存して閉じてから開く', async () => {
    const { compose } = setup()
    await openReply(compose)
    compose.content.subject = 'edited'
    h.responses.OpenCompose = session({ sessionId: 's2' })
    expect(await compose.open({ mode: 'forward', ref: { accountId: 'a1', threadId: 'u' } })).toBe(
      true,
    )
    expect(names()).toEqual(['OpenCompose', 'SaveDraft', 'CloseCompose', 'OpenCompose'])
    expect(compose.session?.sessionId).toBe('s2')
  })
})

describe('close / discard / save', () => {
  test('未保存があれば保存してから閉じる', async () => {
    const { compose } = setup()
    await openReply(compose)
    compose.content.subject = 'edited'
    expect(await compose.close()).toBe(true)
    expect(names()).toEqual(['OpenCompose', 'SaveDraft', 'CloseCompose'])
    expect(compose.session).toBeUndefined()
  })

  test('未編集なら保存せず閉じる', async () => {
    const { compose } = setup()
    await openReply(compose)
    await compose.close()
    expect(names()).toEqual(['OpenCompose', 'CloseCompose'])
  })

  test('保存に失敗したら閉じず、内容を保持してエラーを出す', async () => {
    const { compose } = setup()
    await openReply(compose)
    compose.content.subject = 'edited'
    h.failures.SaveDraft = appError('OFFLINE', { retryable: true })
    expect(await compose.close()).toBe(false)
    expect(compose.session).toBeDefined()
    expect(compose.content.subject).toBe('edited')
    expect(compose.error).toContain('閉じませんでした')
    expect(count('CloseCompose')).toBe(0)
  })

  test('送信元未選択の新規作成は保存せず閉じる（メモリのみ）', async () => {
    const { compose } = setup()
    await openReply(compose, {
      mode: 'new',
      senderLocked: false,
      accountId: undefined,
      from: undefined,
    })
    compose.content.subject = 'memo'
    expect(await compose.close()).toBe(true)
    expect(count('SaveDraft')).toBe(0)
  })

  test('discard は DiscardCompose を呼んでセッションを破棄する', async () => {
    const { compose } = setup()
    await openReply(compose)
    compose.content.subject = 'edited'
    expect(await compose.discard()).toBe(true)
    expect(count('DiscardCompose')).toBe(1)
    expect(count('SaveDraft')).toBe(0)
    expect(compose.session).toBeUndefined()
  })

  test('discard が失敗したら閉じない', async () => {
    const { compose } = setup()
    await openReply(compose)
    h.failures.DiscardCompose = appError('GMAIL_ERROR')
    expect(await compose.discard()).toBe(false)
    expect(compose.session).toBeDefined()
    expect(compose.error).toBeDefined()
  })

  test('save / flush は SaveDraft を呼び、dirty を戻して draftId を保持する', async () => {
    const { compose } = setup()
    await openReply(compose)
    compose.content.subject = 'edited'
    expect(compose.dirty).toBe(true)
    expect(await compose.flush()).toBe(true)
    expect(compose.dirty).toBe(false)
    expect(compose.session?.draftId).toBe('d1')
    expect(count('SaveDraft')).toBe(1)
    expect(await compose.save()).toBe(true)
    expect(count('SaveDraft')).toBe(2)
  })
})

describe('添付と AI 下書き', () => {
  test('AddAttachments は一覧を置き換え、上限超過はエラー表示で一覧を変えない', async () => {
    const { compose } = setup()
    await openReply(compose)
    h.responses.AddAttachments = [{ id: 'f', filename: 'a', size: 1, source: 'local' }]
    expect(await compose.addAttachments(['/tmp/a'])).toBe(true)
    expect(compose.attachments).toHaveLength(1)
    h.failures.AddAttachments = appError('ATTACHMENT_TOO_LARGE', {
      details: { limitBytes: 26214400 },
    })
    expect(await compose.addAttachments(['/tmp/b'])).toBe(false)
    expect(compose.attachments).toHaveLength(1)
    expect(compose.attachmentError).toContain('上限')
    expect(compose.attachmentError).toContain('25')
  })

  test('insertGenerated は署名と引用を保って本文だけ置き換える', async () => {
    const { compose } = setup()
    await openReply(compose)
    compose.insertGenerated('<h1>見出し</h1><p>AI の返信</p>')
    const html = compose.content.bodyHtml
    expect(html).toContain('AI の返信')
    expect(html).not.toContain('<h1')
    expect(findSignature(html)?.accountId).toBe('a1')
    expect(html).toContain('data-hitomado-quote')
    expect(compose.dirty).toBe(true)
  })
})
