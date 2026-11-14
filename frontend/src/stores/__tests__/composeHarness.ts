// composeStore のテスト用ハーネス: バインディングをモックし、呼び出しを記録する。
import { expect, mock } from 'bun:test'
import { createPinia, setActivePinia } from 'pinia'
import * as realAccount from '../../../bindings/github.com/ousiassllc/hitomado/internal/app/accountservice'
import * as realApp from '../../../bindings/github.com/ousiassllc/hitomado/internal/app/appservice'
import * as realCompose from '../../../bindings/github.com/ousiassllc/hitomado/internal/app/composeservice'
import type {
  ComposeSession,
  SendPreview,
} from '../../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import * as realInbox from '../../../bindings/github.com/ousiassllc/hitomado/internal/app/inboxservice'
import { buildSignatureBlock } from '../../lib/signature'

const B = '../../../bindings/github.com/ousiassllc/hitomado/internal/app'

export const calls: { name: string; args: unknown[] }[] = []
/** 可変のモック状態（テストごとに reset で初期化）。 */
export const h: {
  responses: Record<string, unknown>
  failures: Record<string, unknown>
  gates: Record<string, Promise<void>>
} = { responses: {}, failures: {}, gates: {} }

function binding(name: string) {
  return async (...args: unknown[]) => {
    calls.push({ name, args })
    if (name in h.gates) await h.gates[name]
    if (name in h.failures) throw h.failures[name]
    const r = h.responses[name]
    return typeof r === 'function' ? (r as (...a: unknown[]) => unknown)(...args) : r
  }
}

export function installMocks(): void {
  mock.module(`${B}/composeservice`, () => ({
    OpenCompose: binding('OpenCompose'),
    SetComposeSender: binding('SetComposeSender'),
    AddAttachments: binding('AddAttachments'),
    RemoveAttachment: binding('RemoveAttachment'),
    SaveDraft: binding('SaveDraft'),
    PrepareSend: binding('PrepareSend'),
    Send: binding('Send'),
    CloseCompose: binding('CloseCompose'),
    DiscardCompose: binding('DiscardCompose'),
    SuggestContacts: binding('SuggestContacts'),
    ListDrafts: binding('ListDrafts'),
    DiscardDraft: binding('DiscardDraft'),
  }))
  mock.module(`${B}/accountservice`, () => ({}))
  mock.module(`${B}/appservice`, () => ({
    GetInboxFilter: binding('GetInboxFilter'),
    SetInboxFilter: binding('SetInboxFilter'),
  }))
  mock.module(`${B}/inboxservice`, () => ({
    ListInbox: binding('ListInbox'),
    GetUnreadCounts: binding('GetUnreadCounts'),
  }))
}

export function restoreMocks(): void {
  mock.module(`${B}/composeservice`, () => ({ ...realCompose }))
  mock.module(`${B}/accountservice`, () => ({ ...realAccount }))
  mock.module(`${B}/appservice`, () => ({ ...realApp }))
  mock.module(`${B}/inboxservice`, () => ({ ...realInbox }))
}

installMocks()

export const { useComposeStore } = await import('../compose')
export const { useUiStore } = await import('../ui')
export const { useAccountsStore } = await import('../accounts')
export const { useInboxStore } = await import('../inbox')
export const { useDraftsStore } = await import('../drafts')
export const { useSettingsStore } = await import('../settings')

export const SIG_A = '<p>Aの署名</p>'
export const SIG_B = '<p>Bの署名</p>'

export function session(over: Partial<ComposeSession> = {}): ComposeSession {
  return {
    sessionId: 's1',
    mode: 'reply',
    from: { email: 'me@example.com' },
    accountId: 'a1',
    senderLocked: true,
    content: {
      to: [{ email: 'to@example.com' }],
      cc: null,
      bcc: null,
      subject: 'Re: hi',
      bodyHtml: `<p></p>${buildSignatureBlock('a1', SIG_A)}<blockquote data-hitomado-quote><p>q</p></blockquote>`,
    },
    attachments: null,
    ...over,
  }
}

export function previewOf(token = 't1'): SendPreview {
  return {
    confirmToken: token,
    expiresAt: 0,
    from: { email: 'me@example.com', accountId: 'a1', color: 'red' },
    to: [{ email: 'to@example.com' }],
    cc: null,
    bcc: null,
    subject: 'Re: hi',
    attachmentCount: 0,
    bodyPreview: '',
    warnings: null,
  }
}

export function appError(code: string, extra: Record<string, unknown> = {}) {
  return { code, message: `msg-${code}`, retryable: false, ...extra }
}

export const names = () => calls.map((c) => c.name)
export const count = (n: string) => calls.filter((c) => c.name === n).length

export function setup() {
  setActivePinia(createPinia())
  const accounts = useAccountsStore()
  accounts.accounts = [
    { id: 'a1', signatureHtml: SIG_A, email: 'a1@example.com', displayName: 'A', color: 'red' },
    { id: 'a2', signatureHtml: SIG_B, email: 'a2@example.com', displayName: 'B', color: 'blue' },
    { id: 'a3', signatureHtml: '', email: 'a3@example.com', displayName: 'C', color: 'green' },
  ] as typeof accounts.accounts
  return { compose: useComposeStore(), ui: useUiStore() }
}

export async function openReply(
  compose: ReturnType<typeof useComposeStore>,
  over: Partial<ComposeSession> = {},
) {
  h.responses.OpenCompose = session(over)
  expect(await compose.open({ mode: 'reply', ref: { accountId: 'a1', threadId: 't' } })).toBe(true)
}

export function reset(): void {
  calls.length = 0
  h.failures = {}
  h.gates = {}
  h.responses = {
    SaveDraft: { draftId: 'd1', savedAt: 1 },
    CloseCompose: undefined,
    DiscardCompose: undefined,
    ListInbox: { items: [], failures: [] },
    GetUnreadCounts: {},
    PrepareSend: previewOf(),
    Send: { messageId: 'm', threadId: 't', archived: false },
    SetComposeSender: (_sid: string, accountId: string) =>
      session({ mode: 'new', senderLocked: false, accountId }),
  }
}
