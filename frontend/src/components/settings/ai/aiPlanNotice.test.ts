import { beforeEach, describe, expect, test } from 'bun:test'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import type { ChatGPTAccount } from '../../../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import { useSettingsStore } from '../../../stores/settings'
import { acctA, acctB, base, chatgptCfg, setup } from './aiTestSupport'
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

  test('Got it で確認を記録して閉じる。Manage usage は ChatGPT の使用状況の画面を開く', async () => {
    const { w, opened, acked } = mountNotice([pending(acctA)])
    await flushPromises()
    document.body
      .querySelector<HTMLButtonElement>('[data-testid="ai-chatgpt-notice-manage"]')
      ?.click()
    expect(opened).toEqual(['https://chatgpt.com/settings/usage'])
    expect(acked).toEqual([])

    document.body
      .querySelector<HTMLButtonElement>('[data-testid="ai-chatgpt-notice-gotit"]')
      ?.click()
    await flushPromises()
    expect(acked).toEqual(['oaiapp_AAAA'])
    expect(dialogText()).not.toContain("You're using your ChatGPT plan")
    w.unmount()
  })

  test('確認の記録に失敗しても、押したあとに出し続けない（トーストで知らせる）', async () => {
    const { store } = mountNotice([pending(acctA)])
    store.ackChatGPTPlanNotice = async () => {
      throw { code: 'INTERNAL', message: '保存に失敗しました', retryable: false }
    }
    await flushPromises()
    document.body
      .querySelector<HTMLButtonElement>('[data-testid="ai-chatgpt-notice-gotit"]')
      ?.click()
    await flushPromises()
    expect(dialogText()).not.toContain("You're using your ChatGPT plan")
  })

  test('2 つ目のアカウントを初めてログインしたときだけ、もう一度出る（1 つ目は確認済みのまま）', async () => {
    setActivePinia(createPinia())
    const { pinia, store } = setup(chatgptCfg([acctA]))
    void pinia
    const acked: string[] = []
    store.ackChatGPTPlanNotice = async (id: string) => {
      acked.push(id)
      store.aiConfig = {
        ...base,
        ...chatgptCfg([acctA, { ...acctB, active: true, planNoticePending: false }]),
      }
    }
    const w = mount(ChatGPTPlanNotice, {
      props: { accounts: [acctA, pending({ ...acctB, active: true })] },
      attachTo: document.body,
    })
    await flushPromises()
    expect(dialogText()).toContain('b@chatgpt.example')
    expect(dialogText()).not.toContain('a@chatgpt.example')
    document.body
      .querySelector<HTMLButtonElement>('[data-testid="ai-chatgpt-notice-gotit"]')
      ?.click()
    await flushPromises()
    expect(acked).toEqual(['oaiapp_BBBB'])
    w.unmount()
  })
})
