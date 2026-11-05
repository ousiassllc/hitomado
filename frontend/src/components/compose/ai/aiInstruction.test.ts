import { beforeEach, describe, expect, test } from 'bun:test'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { type AiDraftDeps, useAiDraft } from '../../../composables/useAiDraft'
import { instructionLength, MAX_INSTRUCTION_CHARS } from '../../../lib/aiInstruction'
import { compose as msg } from '../../../lib/messages'
import { useComposeStore } from '../../../stores/compose'
import { useSettingsStore } from '../../../stores/settings'
import AiDraftButton from './AiDraftButton.vue'
import AiDraftInstruction from './AiDraftInstruction.vue'

beforeEach(() => {
  document.body.innerHTML = ''
  setActivePinia(createPinia())
})

const el = (testid: string) => document.body.querySelector<HTMLElement>(`[data-testid="${testid}"]`)

function mountButton(consented = true) {
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
  const calls: { sessionId: string; instruction: string | undefined }[] = []
  settings.generateReplyDraft = async (
    sessionId: string,
    _requestId: string,
    instruction?: string,
  ) => {
    calls.push({ sessionId, instruction })
    return { bodyHtml: '<p>x</p>' }
  }
  const w = mount(AiDraftButton, { props: { sessionId: 's1' }, attachTo: document.body })
  return { w, calls, compose: useComposeStore() }
}

describe('instructionLength', () => {
  test('前後の空白を除いたコードポイント数で数える（Go と同じ。サロゲートペアは 1 文字）', () => {
    expect(instructionLength('')).toBe(0)
    expect(instructionLength('  \n ')).toBe(0)
    expect(instructionLength(' あい ')).toBe(2)
    expect(instructionLength('😀😀')).toBe(2)
    expect(MAX_INSTRUCTION_CHARS).toBe(2000)
  })
})

describe('AiDraftInstruction（入力欄）', () => {
  test('入力は v-model で返り、残り文字数を出す。上限を超えると警告の色になる', async () => {
    const w = mount(AiDraftInstruction, {
      props: {
        modelValue: '',
        'onUpdate:modelValue': (v: string) => w.setProps({ modelValue: v }),
      },
    })
    expect(w.get('[data-testid="ai-instruction-remaining"]').text()).toBe(
      msg.ai.instruction.remaining(2000),
    )
    await w.get('textarea').setValue('あ'.repeat(30))
    expect(w.props('modelValue')).toBe('あ'.repeat(30))
    expect(w.get('[data-testid="ai-instruction-remaining"]').text()).toBe('残り 1970 文字')
    expect(w.get('[data-testid="ai-instruction-remaining"]').classes()).not.toContain('over')
    await w.get('textarea').setValue('あ'.repeat(2001))
    expect(w.get('[data-testid="ai-instruction-remaining"]').text()).toBe('残り -1 文字')
    expect(w.get('[data-testid="ai-instruction-remaining"]').classes()).toContain('over')
  })

  test('生成中は入力できない', () => {
    const w = mount(AiDraftInstruction, { props: { modelValue: '', disabled: true } })
    expect(w.get('textarea').attributes('disabled')).toBeDefined()
  })
})

describe('AiDraftButton の「指示を追加」', () => {
  test('初期は閉じていて、「指示を追加」で開閉する。入力は作成セッション（composeStore）に入る', async () => {
    const { w, compose } = mountButton()
    expect(el('ai-instruction')).toBeNull()
    const toggle = el('ai-instruction-toggle')
    expect(toggle?.textContent?.trim()).toBe('指示を追加')
    toggle?.click()
    await flushPromises()
    expect(el('ai-instruction')).not.toBeNull()
    expect(toggle?.textContent?.trim()).toBe('指示を閉じる')
    const input = el('ai-instruction-input') as HTMLTextAreaElement
    input.value = '丁寧に断る'
    input.dispatchEvent(new Event('input'))
    await flushPromises()
    expect(compose.aiInstruction).toBe('丁寧に断る')

    toggle?.click()
    await flushPromises()
    expect(el('ai-instruction')).toBeNull()
    expect(compose.aiInstruction).toBe('丁寧に断る') // 閉じても値は残る
    expect(toggle?.textContent?.trim()).toBe('指示を編集（入力あり）')
    w.unmount()
  })

  test('生成には入力した指示を渡す。未入力は空文字（指示なし）。生成後も入力欄に残る', async () => {
    const { w, calls, compose } = mountButton()
    await w.get('button').trigger('click')
    await flushPromises()
    expect(calls).toEqual([{ sessionId: 's1', instruction: '' }])

    compose.aiInstruction = '日程は来週以降で'
    await w.get('button').trigger('click')
    await flushPromises()
    expect(calls[1]).toEqual({ sessionId: 's1', instruction: '日程は来週以降で' })
    expect(compose.aiInstruction).toBe('日程は来週以降で')
    w.unmount()
  })

  test('2,000 文字を超える指示は、同意の確認も生成も始めずにエラーを出す（本文は変えない）', async () => {
    const { w, calls, compose } = mountButton(false)
    compose.aiInstruction = 'あ'.repeat(2001)
    await w.get('button').trigger('click')
    await flushPromises()
    expect(calls).toEqual([])
    expect(el('ai-consent-body')).toBeNull()
    const alert = w.get('.ai-error')
    expect(alert.text()).toContain(msg.ai.instruction.tooLong(2000))
    expect(alert.text()).toContain('本文は変更していません')

    compose.aiInstruction = 'あ'.repeat(2000)
    await w.get('button').trigger('click')
    await flushPromises()
    expect(w.find('.ai-error').exists()).toBe(false)
    expect(el('ai-consent-body')).not.toBeNull() // 上限内なら、通常どおり同意の確認へ進む
    w.unmount()
  })

  test('同意の画面に「入力した指示も送信される」と出る', async () => {
    const { w } = mountButton(false)
    await w.get('button').trigger('click')
    await flushPromises()
    expect(el('ai-consent-body')?.textContent).toContain('指示も送信されます')
    w.unmount()
  })
})

describe('useAiDraft の instructionError', () => {
  test('指定が無ければ従来どおり生成する', async () => {
    const generated: string[] = []
    const deps: AiDraftDeps = {
      config: () => ({
        provider: 'anthropic',
        configured: true,
        hasApiKey: true,
        consentedProviders: ['anthropic'],
      }),
      hasBody: () => false,
      recordConsent: async () => {},
      generate: async (id) => {
        generated.push(id)
        return { bodyHtml: '' }
      },
      cancel: async () => {},
      openSetup: () => {},
      onGenerated: () => {},
      newRequestId: () => 'r1',
    }
    await useAiDraft(deps).start()
    expect(generated).toEqual(['r1'])
  })
})
