import { mock } from 'bun:test'
import { createPinia, setActivePinia } from 'pinia'
import * as realIndex from '../../../bindings/github.com/ousiassllc/hitomado/internal/app'
import type {
  InboxZeroCandidate,
  ThreadSummary,
} from '../../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import { features } from '../../../test/aiConfigKit'
import {
  callsOf,
  ctl,
  installMocks,
  calls as kitCalls,
  resetState,
  restoreMocks,
} from '../../components/thread/threadTestKit'

// Inbox Zero のストアのテストの共通部分（#314・#377）。選んだ AI（Jev・OpenAI など）の実 API は呼ばない。ストア（inboxZero.ts）が import する Go のバインディング（index の AIService）をモックする。
// 同意の保存（settings.recordAIConsent）は、すでに読み込まれた settings ストアのメソッドを setup で差し替える。
const B = '../../../bindings/github.com/ousiassllc/hitomado/internal/app'
installMocks()
export const aiCtl: { find: unknown; failures: Record<string, unknown> } = {
  find: undefined,
  failures: {},
}
function aiBinding(name: string) {
  return async (...args: unknown[]) => {
    kitCalls.push({ name, args })
    if (name in aiCtl.failures) throw aiCtl.failures[name]
    return typeof aiCtl.find === 'function'
      ? (aiCtl.find as (...a: unknown[]) => unknown)(...args)
      : aiCtl.find
  }
}
mock.module(B, () => ({
  ...realIndex,
  AIService: {
    FindInboxZeroCandidates: aiBinding('FindInboxZeroCandidates'),
    CancelGenerate: aiBinding('CancelGenerate'),
  },
}))
/** テストファイルの afterAll で呼ぶ（モックを元に戻す）。 */
export function restoreInboxZeroMocks(): void {
  restoreMocks()
  mock.module(B, () => ({ ...realIndex }))
}

const { useInboxStore } = await import('../inbox')
const { useInboxZeroStore } = await import('../inboxZero')
const { useSettingsStore } = await import('../settings')
const { useUiStore } = await import('../ui')

export { INBOX_ZERO_MAX } from '../inboxZero'

function summary(n: number): ThreadSummary {
  return {
    accountId: 'a1',
    threadId: `t${n}`,
    subject: `件名 ${n}`,
    snippet: '',
    lastMessageAt: 1000 - n,
    lastFrom: { email: `x${n}@example.com` },
    messageCount: 1,
    isUnread: true,
    hasAttachment: false,
    inInbox: true,
    labels: null,
  }
}

export const cand = (n: number, reason = 'ad'): InboxZeroCandidate => ({
  ref: { accountId: 'a1', threadId: `t${n}` },
  from: `送信者 ${n}`,
  subject: `件名 ${n}`,
  reason,
})

export const ALL_ON = {
  ad: true,
  notification: true,
  newsletter: true,
  replied: true,
  calendar: true,
}
export const NONE = {
  ad: false,
  notification: false,
  newsletter: false,
  replied: false,
  calendar: false,
}
export const AI_OFF = { ...NONE, replied: true, calendar: true }

// 既定は、Inbox Zero の AI が未選択で Jev のキーだけがある状態（#314 からの互換）。selected で Inbox Zero に AI を選び、
// providers でつないである API キーのプロバイダ（同意済みかは consented）を変える。
export async function setup(
  opts: {
    count?: number
    consented?: boolean
    keyed?: boolean
    selected?: string
    providers?: string[]
    baseUrl?: string
  } = {},
) {
  const { count = 3, consented = true, keyed = true, selected = '', baseUrl } = opts
  const providers = opts.providers ?? (keyed ? ['typesafe'] : [])
  setActivePinia(createPinia())
  resetState()
  aiCtl.find = undefined
  aiCtl.failures = {}
  const all = Array.from({ length: count }, (_, i) => summary(i + 1))
  // アーカイブしたものは、あとの再取得（アーカイブ後の reload）では返らない。
  ctl.responses.ListInbox = () => ({
    items: all.filter(
      (t) =>
        !callsOf('Archive').some((c) =>
          (c.args[0] as { threadId: string }[]).some((r) => r.threadId === t.threadId),
        ),
    ),
    failures: [],
  })
  ctl.responses.Archive = []
  ctl.responses.Unarchive = [summary(1)]
  const inbox = useInboxStore()
  await inbox.load()
  const settings = useSettingsStore()
  useInboxZeroStore().reasons = { ...ALL_ON }
  settings.aiConfig = {
    features: features(
      '',
      selected === '' ? '' : { provider: selected, model: selected === 'typesafe' ? '' : 'gpt-x' },
    ),
    apiKeyProviders: providers,
    consentedProviders: consented ? providers : [],
    baseUrl,
    chatgptSignedIn: false,
    chatgptAccounts: [],
  }
  settings.recordAIConsent = async (provider: string, baseUrl = '') => {
    kitCalls.push({ name: 'RecordAIConsent', args: [provider, baseUrl] })
    if (settings.aiConfig) {
      settings.aiConfig = { ...settings.aiConfig, consentedProviders: [provider] }
    }
  }
  return { inbox, settings, zero: useInboxZeroStore(), ui: useUiStore() }
}
