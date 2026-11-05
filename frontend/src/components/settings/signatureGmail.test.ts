import { afterAll, beforeAll, beforeEach, describe, expect, mock, test } from 'bun:test'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import * as realAccount from '../../../bindings/github.com/ousiassllc/hitomado/internal/app/accountservice'
import type { Account } from '../../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import { settings as msg } from '../../lib/messages'

const B = '../../../bindings/github.com/ousiassllc/hitomado/internal/app'

const calls: { name: string; args: unknown[] }[] = []
const ctl: { responses: Record<string, unknown>; failures: Record<string, unknown> } = {
  responses: {},
  failures: {},
}

function binding(name: string) {
  return async (...args: unknown[]) => {
    calls.push({ name, args })
    if (name in ctl.failures) throw ctl.failures[name]
    return ctl.responses[name]
  }
}

function account(over: Partial<Account> = {}): Account {
  return {
    id: 'a1',
    oauthClientId: 'c1',
    email: 'me@a.example',
    displayName: 'Me',
    color: 'blue',
    signatureHtml: '',
    notifyEnabled: true,
    status: 'active',
    sortOrder: 0,
    ...over,
  }
}

let SignatureSettings: typeof import('./SignatureSettings.vue').default
let useAccountsStore: typeof import('../../stores/accounts').useAccountsStore
let useUiStore: typeof import('../../stores/ui').useUiStore

beforeAll(async () => {
  mock.module(`${B}/accountservice`, () => ({
    ...realAccount,
    FetchGmailSignature: binding('FetchGmailSignature'),
    SetSignature: binding('SetSignature'),
  }))
  SignatureSettings = (await import('./SignatureSettings.vue')).default
  useAccountsStore = (await import('../../stores/accounts')).useAccountsStore
  useUiStore = (await import('../../stores/ui')).useUiStore
})

afterAll(() => {
  mock.module(`${B}/accountservice`, () => ({ ...realAccount }))
})

beforeEach(() => {
  calls.length = 0
  ctl.responses = {}
  ctl.failures = {}
  document.body.innerHTML = ''
})

async function mountWith(
  acc: Account,
): Promise<{ w: VueWrapper; ui: ReturnType<typeof useUiStore> }> {
  const pinia = createPinia()
  setActivePinia(pinia)
  useAccountsStore().accounts = [acc]
  const w = mount(SignatureSettings, { global: { plugins: [pinia] }, attachTo: document.body })
  return { w, ui: useUiStore() }
}

function fetchButton(w: VueWrapper) {
  return w.findAll('button').find((b) => b.text() === msg.signature.fetchGmail)
}

function editorText(w: VueWrapper): string {
  return w.get('[role="textbox"]').text()
}

function toasts(ui: ReturnType<typeof useUiStore>): string[] {
  return ui.toasts.map((t) => t.message)
}

function dialogButton(label: string): HTMLButtonElement | undefined {
  return [...document.body.querySelectorAll<HTMLButtonElement>('[role="dialog"] button')].find(
    (b) => b.textContent?.trim() === label,
  )
}

describe('署名の「Gmail から取得」', () => {
  test('エディタが空なら確認なしで入り、保存はしない', async () => {
    ctl.responses.FetchGmailSignature = {
      signatureHtml: '<p>山田 太郎</p>',
      found: true,
      sanitized: false,
    }
    const { w, ui } = await mountWith(account())
    await fetchButton(w)?.trigger('click')
    await flushPromises()

    expect(editorText(w)).toBe('山田 太郎')
    expect(document.body.querySelector('[role="dialog"]')).toBeNull()
    expect(calls.map((c) => c.name)).toEqual(['FetchGmailSignature'])
    expect(calls[0]?.args).toEqual(['a1'])
    expect(toasts(ui)).toEqual([msg.signature.fetchedGmail('Me')])
    expect(w.text()).toContain(msg.signature.dirty)
    w.unmount()
  })

  test('取り込めない部分を除いたときは、その旨を表示する', async () => {
    ctl.responses.FetchGmailSignature = {
      signatureHtml: '<p>Taro</p>',
      found: true,
      sanitized: true,
    }
    const { w, ui } = await mountWith(account())
    await fetchButton(w)?.trigger('click')
    await flushPromises()

    expect(editorText(w)).toBe('Taro')
    expect(toasts(ui)).toContain(msg.signature.fetchSanitized)
    expect(msg.signature.fetchSanitized).toBe('画像・装飾など、取り込めない部分は除きました。')
    w.unmount()
  })

  test('入力済みなら置き換える前に確認し、キャンセルならエディタを変えない', async () => {
    ctl.responses.FetchGmailSignature = {
      signatureHtml: '<p>Gmail 署名</p>',
      found: true,
      sanitized: false,
    }
    const { w, ui } = await mountWith(account({ signatureHtml: '<p>既存の署名</p>' }))
    await fetchButton(w)?.trigger('click')
    await flushPromises()

    expect(document.body.querySelector('[role="dialog"]')).not.toBeNull()
    expect(editorText(w)).toBe('既存の署名')
    dialogButton(msg.signature.replaceCancel)?.click()
    await flushPromises()

    expect(editorText(w)).toBe('既存の署名')
    expect(document.body.querySelector('[role="dialog"]')).toBeNull()
    expect(toasts(ui)).toEqual([])
    w.unmount()
  })

  test('確認で「置き換える」を選ぶとエディタに入る（まだ保存しない）', async () => {
    ctl.responses.FetchGmailSignature = {
      signatureHtml: '<p>Gmail 署名</p>',
      found: true,
      sanitized: false,
    }
    const { w } = await mountWith(account({ signatureHtml: '<p>既存の署名</p>' }))
    await fetchButton(w)?.trigger('click')
    await flushPromises()
    dialogButton(msg.signature.replaceConfirm)?.click()
    await flushPromises()

    expect(editorText(w)).toBe('Gmail 署名')
    expect(calls.map((c) => c.name)).toEqual(['FetchGmailSignature'])
    w.unmount()
  })

  test('Gmail の署名が空ならエディタを変えず、その旨を表示する', async () => {
    ctl.responses.FetchGmailSignature = { signatureHtml: '', found: false, sanitized: false }
    const { w, ui } = await mountWith(account({ signatureHtml: '<p>既存の署名</p>' }))
    await fetchButton(w)?.trigger('click')
    await flushPromises()

    expect(editorText(w)).toBe('既存の署名')
    expect(document.body.querySelector('[role="dialog"]')).toBeNull()
    expect(toasts(ui)).toEqual(['Gmail に署名が設定されていません。'])
    w.unmount()
  })

  test('取得に失敗したらエディタを変えず、理由を表示する', async () => {
    ctl.failures.FetchGmailSignature = {
      code: 'OFFLINE',
      message: 'オフラインです。接続を確認してください。',
      retryable: true,
    }
    const { w, ui } = await mountWith(account({ signatureHtml: '<p>既存の署名</p>' }))
    await fetchButton(w)?.trigger('click')
    await flushPromises()

    expect(editorText(w)).toBe('既存の署名')
    expect(document.body.querySelector('[role="dialog"]')).toBeNull()
    expect(toasts(ui)).toEqual(['オフラインです。接続を確認してください。'])
    expect(fetchButton(w)?.attributes('disabled')).toBeUndefined()
    w.unmount()
  })

  test('取得中はボタンを無効にし、二重に呼ばない', async () => {
    let release: (v: unknown) => void = () => {}
    ctl.responses.FetchGmailSignature = new Promise((r) => {
      release = r
    })
    const { w } = await mountWith(account())
    const btn = fetchButton(w)
    await btn?.trigger('click')
    await btn?.trigger('click')
    expect(fetchButton(w)?.attributes('disabled')).toBeDefined()
    release({ signatureHtml: '<p>x</p>', found: true, sanitized: false })
    await flushPromises()

    expect(calls.filter((c) => c.name === 'FetchGmailSignature')).toHaveLength(1)
    expect(fetchButton(w)?.attributes('disabled')).toBeUndefined()
    w.unmount()
  })
})
