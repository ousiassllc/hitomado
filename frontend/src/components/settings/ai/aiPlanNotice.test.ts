import { beforeEach, describe, expect, test } from 'bun:test'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import type { ChatGPTAccount } from '../../../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import { useSettingsStore } from '../../../stores/settings'
import { useUiStore } from '../../../stores/ui'
import { acctA, acctB } from './aiTestSupport'
import ChatGPTPlanNotice from './ChatGPTPlanNotice.vue'

beforeEach(() => {
  document.body.innerHTML = ''
})

const pending = (a: ChatGPTAccount): ChatGPTAccount => ({ ...a, planNoticePending: true })
const dialogText = () => document.body.textContent ?? ''

function mountNotice(accounts: ChatGPTAccount[]) {
  setActivePinia(createPinia())
  const store = useSettingsStore()
  const opened: string[] = []
  store.openExternal = async (url: string) => {
    opened.push(url)
  }
  const acked: string[] = []
  store.ackChatGPTPlanNotice = async (id: string) => {
    acked.push(id)
  }
  const w = mount(ChatGPTPlanNotice, { props: { accounts }, attachTo: document.body })
  return { w, store, opened, acked }
}

describe('ChatGPT プランの確認モーダル（#80）', () => {
  test('初めてログインした（確認が済んでいない）アカウントがあると、文言どおりに出す', async () => {
    mountNotice([pending(acctA)])
    await flushPromises()
    expect(dialogText()).toContain("You're using your ChatGPT plan")
    expect(dialogText()).toContain('will use your ChatGPT plan')
    expect(dialogText()).toContain('manage usage in ChatGPT settings')
    expect(dialogText()).toContain('a@chatgpt.example')
    expect(
      document.body.querySelector('[data-testid="ai-chatgpt-notice-gotit"]')?.textContent?.trim(),
    ).toBe('Got it')
  })

  test('2 回目以降のログイン（確認済み）・ログアウト済み・アカウントなしでは出さない', async () => {
    for (const accounts of [[acctA], [{ ...pending(acctA), signedIn: false }], []]) {
      document.body.innerHTML = ''
      mountNotice(accounts)
      await flushPromises()
      expect(dialogText()).not.toContain("You're using your ChatGPT plan")
    }
  })

  test('Got it で確認を記録して閉じる。「Manage usage」は出さない', async () => {
    const { w, opened, acked } = mountNotice([pending(acctA)])
    await flushPromises()
    expect(dialogText()).not.toContain('Manage usage')
    expect(document.body.querySelector('[data-testid="ai-chatgpt-notice-manage"]')).toBeNull()
    expect(opened).toEqual([])
    expect(acked).toEqual([])

    document.body
      .querySelector<HTMLButtonElement>('[data-testid="ai-chatgpt-notice-gotit"]')
      ?.click()
    await flushPromises()
    expect(acked).toEqual(['oaiapp_AAAA'])
    expect(dialogText()).not.toContain("You're using your ChatGPT plan")
    w.unmount()
  })

  test('Esc で閉じる。確認は記録しない（記録するのは Got it だけ）', async () => {
    const { w, acked } = mountNotice([pending(acctA)])
    await flushPromises()
    document
      .querySelector('[role="dialog"]')
      ?.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    await flushPromises()
    expect(acked).toEqual([])
    expect(dialogText()).not.toContain("You're using your ChatGPT plan")
    w.unmount()
  })

  test('外側（スクラム）クリックで閉じる。確認は記録せず、× のボタンも出る', async () => {
    const { w, acked } = mountNotice([pending(acctA)])
    await flushPromises()
    expect(document.body.querySelector('button[aria-label="閉じる"]')).not.toBeNull()
    // Reka は pointerdown の購読を setTimeout(0) で始める。その購読が済んでから押す。
    await new Promise((r) => setTimeout(r, 0))
    document
      .querySelector('.dlg-overlay')
      ?.dispatchEvent(new Event('pointerdown', { bubbles: true }))
    await flushPromises()
    expect(acked).toEqual([])
    expect(dialogText()).not.toContain("You're using your ChatGPT plan")
    w.unmount()
  })

  test('確認の記録に失敗しても、押したあとに出し続けない（記録できなかったことと次回また出ることをトーストで知らせる）', async () => {
    const { store } = mountNotice([pending(acctA)])
    store.ackChatGPTPlanNotice = async () => {
      throw { code: 'INTERNAL', message: '保存に失敗しました', retryable: false }
    }
    const ui = useUiStore()
    await flushPromises()
    document.body
      .querySelector<HTMLButtonElement>('[data-testid="ai-chatgpt-notice-gotit"]')
      ?.click()
    await flushPromises()
    expect(dialogText()).not.toContain("You're using your ChatGPT plan")
    expect(ui.alerts.map((a) => [a.kind, a.message])).toEqual([
      ['error', '確認の記録に失敗しました。次回もう一度表示されます。'],
    ])
  })

  test('2 つ目のアカウントを初めてログインしたときだけ、もう一度出る（1 つ目は確認済みのまま）', async () => {
    const { w, acked } = mountNotice([pending(acctA)])
    const gotIt = async () => {
      document.body
        .querySelector<HTMLButtonElement>('[data-testid="ai-chatgpt-notice-gotit"]')
        ?.click()
      await flushPromises()
    }
    await flushPromises()
    expect(dialogText()).toContain('a@chatgpt.example')

    // 1 つ目を確認すると閉じる。
    await gotIt()
    expect(acked).toEqual(['oaiapp_AAAA'])
    expect(dialogText()).not.toContain("You're using your ChatGPT plan")

    // 確認済みになった 1 つ目と、初めてログインした 2 つ目。2 つ目のぶんだけが出る。
    await w.setProps({ accounts: [acctA, pending({ ...acctB, active: true })] })
    await flushPromises()
    expect(dialogText()).toContain('b@chatgpt.example')
    expect(dialogText()).not.toContain('a@chatgpt.example')

    await gotIt()
    expect(acked).toEqual(['oaiapp_AAAA', 'oaiapp_BBBB'])
    expect(dialogText()).not.toContain("You're using your ChatGPT plan")

    // どちらも確認済みなら、もう出ない。
    await w.setProps({ accounts: [acctA, { ...acctB, active: true }] })
    await flushPromises()
    expect(dialogText()).not.toContain("You're using your ChatGPT plan")
    expect(acked).toEqual(['oaiapp_AAAA', 'oaiapp_BBBB'])
    w.unmount()
  })
})
