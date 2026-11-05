import '../../../test/domPurifyShim'
import { afterAll, beforeEach, describe, expect, mock, test } from 'bun:test'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import * as realCompose from '../../../bindings/github.com/ousiassllc/hitomado/internal/app/composeservice'
import { ctl, detail, installMocks, REF, resetState, restoreMocks } from './threadTestKit'

// スレッド内の返信・転送の作成画面を「広げる」（F-11「本文の入力欄の大きさ」。#112）。

const B = '../../../bindings/github.com/ousiassllc/hitomado/internal/app'
const session = {
  sessionId: 's1',
  mode: 'reply',
  accountId: 'a1',
  from: { email: 'me@example.com' },
  senderLocked: true,
  content: {
    to: [{ email: 'to@example.com' }],
    cc: null,
    bcc: null,
    subject: 'Re: s',
    bodyHtml: '<p></p>',
  },
  attachments: null,
}
const composeCalls: string[] = []
const composeBinding = (name: string, result: unknown) => async () => {
  composeCalls.push(name)
  return result
}

installMocks()
mock.module(`${B}/composeservice`, () => ({
  OpenCompose: composeBinding('OpenCompose', session),
  SaveDraft: composeBinding('SaveDraft', { draftId: 'd1', savedAt: 1 }),
  CloseCompose: composeBinding('CloseCompose', undefined),
  DiscardCompose: composeBinding('DiscardCompose', undefined),
  SuggestContacts: composeBinding('SuggestContacts', []),
}))
afterAll(() => {
  restoreMocks()
  mock.module(`${B}/composeservice`, () => ({ ...realCompose }))
})

const { useThreadStore } = await import('../../stores/thread')
const { useComposeStore } = await import('../../stores/compose')
const { default: ThreadPane } = await import('./ThreadPane.vue')

beforeEach(() => {
  document.body.innerHTML = ''
  setActivePinia(createPinia())
  resetState()
  composeCalls.length = 0
})

async function openEditing() {
  ctl.responses.GetThread = detail(false)
  const thread = useThreadStore()
  const compose = useComposeStore()
  const w = mount(ThreadPane, { attachTo: document.body })
  await thread.open(REF)
  await flushPromises()
  await compose.open({ mode: 'reply', ref: REF })
  await flushPromises()
  return { w, thread, compose }
}

const hidden = (el: Element) => (el as HTMLElement).style.display === 'none'

describe('ThreadPane: 作成画面を広げる', () => {
  test('埋め込みの作成画面に「広げる」があり、押すとスレッドの本文を隠し、「元に戻す」で戻る', async () => {
    const { w } = await openEditing()
    const btn = w.get('[data-testid="compose-expand"]')
    expect(btn.text()).toBe('広げる')
    expect(hidden(w.get('[data-testid="thread-scroll"]').element)).toBe(false)
    expect(hidden(w.get('[data-testid="thread-header"]').element)).toBe(false)

    await btn.trigger('click')
    expect(hidden(w.get('[data-testid="thread-scroll"]').element)).toBe(true)
    expect(hidden(w.get('[data-testid="thread-header"]').element)).toBe(true)
    expect(w.get('.compose-pane').classes()).toContain('expanded')
    expect(w.get('[data-testid="compose-expand"]').text()).toBe('元に戻す')

    await w.get('[data-testid="compose-expand"]').trigger('click')
    expect(hidden(w.get('[data-testid="thread-scroll"]').element)).toBe(false)
    expect(hidden(w.get('[data-testid="thread-header"]').element)).toBe(false)
    expect(w.get('.compose-pane').classes()).not.toContain('expanded')
    w.unmount()
  })

  test('広げる前に読んでいた位置（スクロール量）を、元に戻したときに復元する', async () => {
    const { w } = await openEditing()
    const scroll = w.get('[data-testid="thread-scroll"]').element as HTMLElement
    scroll.scrollTop = 120
    await w.get('[data-testid="compose-expand"]').trigger('click')
    scroll.scrollTop = 0 // 非表示のあいだにブラウザが位置を失っても、戻すと復元する
    await w.get('[data-testid="compose-expand"]').trigger('click')
    await flushPromises()
    expect(scroll.scrollTop).toBe(120)
    w.unmount()
  })

  test('閉じる・破棄（送信も同じく作成画面が無くなる）で元に戻り、次に開いた作成画面は広げていない', async () => {
    for (const end of ['close', 'discard'] as const) {
      const { w, compose } = await openEditing()
      await w.get('[data-testid="compose-expand"]').trigger('click')
      expect(w.get('.compose-pane').classes()).toContain('expanded')

      await compose[end]()
      await flushPromises()
      expect(w.find('.compose-pane').exists()).toBe(false)
      expect(hidden(w.get('[data-testid="thread-scroll"]').element)).toBe(false)

      await compose.open({ mode: 'reply', ref: REF })
      await flushPromises()
      expect(w.get('.compose-pane').classes()).not.toContain('expanded')
      expect(w.get('[data-testid="compose-expand"]').text()).toBe('広げる')
      w.unmount()
    }
  })

  test('スレッドを切り替えると、広げた状態は元に戻る', async () => {
    const { w, thread } = await openEditing()
    await w.get('[data-testid="compose-expand"]').trigger('click')
    expect(hidden(w.get('[data-testid="thread-scroll"]').element)).toBe(true)

    ctl.responses.GetThread = detail(false, { threadId: 't2' })
    await thread.open({ accountId: 'a1', threadId: 't2' })
    await flushPromises()
    expect(hidden(w.get('[data-testid="thread-scroll"]').element)).toBe(false)
    w.unmount()
  })

  test('広げた状態は保存しない（作成セッション・下書きの保存内容に含めない）', async () => {
    const { w, compose } = await openEditing()
    await w.get('[data-testid="compose-expand"]').trigger('click')
    expect(compose.dirty).toBe(false)
    expect(composeCalls).toEqual(['OpenCompose'])
    w.unmount()
  })
})
