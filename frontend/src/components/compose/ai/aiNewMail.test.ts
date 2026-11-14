import { beforeEach, describe, expect, test } from 'bun:test'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import type { NewDraftContext } from '../../../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import { compose as msg } from '../../../lib/messages'
import { useComposeStore } from '../../../stores/compose'
import { useSettingsStore } from '../../../stores/settings'
import ComposePane from '../ComposePane.vue'
import AiDraftButton from './AiDraftButton.vue'

beforeEach(() => {
  document.body.innerHTML = ''
  setActivePinia(createPinia())
})

const el = (testid: string) => document.body.querySelector<HTMLElement>(`[data-testid="${testid}"]`)

type NewCall = { sessionId: string; instruction: string; content: NewDraftContext }

function mountNew(consented = true) {
  const settings = useSettingsStore()
  settings.aiConfig = {
    provider: 'anthropic',
    model: 'claude-x',
    configured: true,
    hasApiKey: true,
    apiKeyProviders: ['anthropic'],
    consentedProviders: consented ? ['anthropic'] : [],
    chatgptSignedIn: false,
    chatgptAccounts: [],
  }
  const newCalls: NewCall[] = []
  settings.generateNewDraft = async (sessionId, _requestId, instruction, content) => {
    newCalls.push({ sessionId, instruction, content })
    return { bodyHtml: '<p>生成された本文</p>' }
  }
  const compose = useComposeStore()
  compose.content = {
    to: [{ name: '花子', email: 'hanako@example.com' }],
    cc: [],
    bcc: [],
    subject: '打ち合わせの日程',
    bodyHtml: '',
  }
  const w = mount(AiDraftButton, {
    props: { sessionId: 's1', newMail: true },
    attachTo: document.body,
  })
  return { w, compose, newCalls }
}

const button = (w: ReturnType<typeof mountNew>['w']) => w.get('button')

describe('新規作成の「AI で下書き」（#365）', () => {
  test('指示が空・空白だけなら、生成も同意の確認も始めず、指示を書く案内を出して入力欄へ戻す', async () => {
    for (const blank of ['', '  \n\t ']) {
      const { w, compose, newCalls } = mountNew(false)
      compose.aiInstruction = blank
      await button(w).trigger('click')
      await flushPromises()
      expect(newCalls).toEqual([])
      expect(el('ai-consent-body')).toBeNull()
      expect(w.get('[data-testid="ai-instruction-needed"]').text()).toContain(
        msg.ai.instruction.needed,
      )
      expect(w.find('.ai-error').exists()).toBe(false)
      expect(document.activeElement).toBe(w.get('textarea').element)
      expect(w.get('label').text()).toBe(msg.ai.instruction.labelRequired)
      expect(w.get('textarea').attributes('aria-required')).toBe('true')
      w.unmount()
    }
  })

  test('案内は、指示を入力すると消える。入力して押すと、指示と入力済みの件名・宛先で生成する', async () => {
    const { w, compose, newCalls } = mountNew()
    await button(w).trigger('click')
    await flushPromises()
    expect(w.find('[data-testid="ai-instruction-needed"]').exists()).toBe(true)

    await w.get('textarea').setValue('来週の日程を相談する')
    await flushPromises()
    expect(w.find('[data-testid="ai-instruction-needed"]').exists()).toBe(false)

    await button(w).trigger('click')
    await flushPromises()
    expect(newCalls).toEqual([
      {
        sessionId: 's1',
        instruction: '来週の日程を相談する',
        content: {
          subject: '打ち合わせの日程',
          to: [{ name: '花子', email: 'hanako@example.com' }],
        },
      },
    ])
    expect(w.emitted('generated')?.[0]).toEqual(['<p>生成された本文</p>'])
    expect(compose.aiInstruction).toBe('来週の日程を相談する') // 生成後も入力欄に残る
    w.unmount()
  })

  test('未同意なら、指示を入れたあと同意の確認を挟み、同意するまで生成しない', async () => {
    const { w, compose, newCalls } = mountNew(false)
    compose.aiInstruction = 'お礼を伝える'
    await button(w).trigger('click')
    await flushPromises()
    expect(newCalls).toEqual([])
    expect(el('ai-consent-body')?.textContent).toContain('入力済みの件名・宛先')
    w.unmount()
  })

  test('本文が入力済みなら、置き換える前に確認する。確認するまで生成しない', async () => {
    const { w, compose, newCalls } = mountNew()
    compose.content.bodyHtml = '<p>書きかけの本文</p>'
    compose.aiInstruction = 'お礼を伝える'
    await button(w).trigger('click')
    await flushPromises()
    expect(newCalls).toEqual([])
    const buttons = Array.from(document.body.querySelectorAll('button'))
    expect(document.body.textContent).toContain(msg.ai.replaceTitle)
    buttons.find((b) => b.textContent?.trim() === msg.ai.replaceOk)?.click()
    await flushPromises()
    expect(newCalls).toHaveLength(1)
    w.unmount()
  })

  test('返信では指示は任意のまま（空でも生成する。generateNewDraft は呼ばない）', async () => {
    const settings = useSettingsStore()
    settings.aiConfig = {
      provider: 'anthropic',
      model: 'claude-x',
      configured: true,
      hasApiKey: true,
      apiKeyProviders: ['anthropic'],
      consentedProviders: ['anthropic'],
      chatgptSignedIn: false,
      chatgptAccounts: [],
    }
    const replyCalls: string[] = []
    let newCalls = 0
    settings.generateReplyDraft = async (sessionId) => {
      replyCalls.push(sessionId)
      return { bodyHtml: '<p>返信</p>' }
    }
    settings.generateNewDraft = async () => {
      newCalls++
      return { bodyHtml: '' }
    }
    const w = mount(AiDraftButton, { props: { sessionId: 's9' }, attachTo: document.body })
    expect(w.get('label').text()).toBe(msg.ai.instruction.label)
    await w.get('button').trigger('click')
    await flushPromises()
    expect(replyCalls).toEqual(['s9'])
    expect(newCalls).toBe(0)
    expect(w.find('[data-testid="ai-instruction-needed"]').exists()).toBe(false)
    w.unmount()
  })
})

describe('作成画面（ComposePane）', () => {
  test('新規作成では生成結果が、署名を残して本文に入る', async () => {
    const { compose } = mountNew()
    compose.session = {
      sessionId: 's1',
      mode: 'new',
      accountId: 'a1',
      senderLocked: false,
      content: { to: [], cc: [], bcc: [], subject: '', bodyHtml: '' },
      attachments: [],
    }
    compose.content.bodyHtml = '<p></p><div data-hitomado-signature="a1"><p>署名</p></div>'
    const w = mount(ComposePane, { attachTo: document.body })
    await flushPromises()
    w.findComponent(AiDraftButton).vm.$emit('generated', '<p>生成された本文</p>')
    await flushPromises()
    expect(compose.content.bodyHtml).toContain('生成された本文')
    expect(compose.content.bodyHtml).toContain('署名')
    expect(compose.content.bodyHtml.indexOf('生成された本文')).toBeLessThan(
      compose.content.bodyHtml.indexOf('署名'),
    )
    expect(w.findComponent(AiDraftButton).props('newMail')).toBe(true)
    w.unmount()
  })
})
