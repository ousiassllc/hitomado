import { describe, expect, test } from 'bun:test'
import { mount } from '@vue/test-utils'
import ThreadAiQuestionForm from './ThreadAiQuestionForm.vue'

const tid = (id: string) => `[data-testid="${id}"]`

function mountForm(initial = '', disabled = false) {
  const submits: number[] = []
  const w = mount(ThreadAiQuestionForm, {
    props: {
      modelValue: initial,
      disabled,
      'onUpdate:modelValue': (v: string) => w.setProps({ modelValue: v }),
      onSubmit: () => submits.push(1),
    },
  })
  return { w, submits }
}

describe('ThreadAiQuestionForm（F-18）', () => {
  test('Ctrl+Enter・Cmd+Enter で送り、Enter だけでは送らない（改行）', async () => {
    const { w, submits } = mountForm('期限は？')
    const input = w.get(tid('thread-ai-question-input'))
    await input.trigger('keydown', { key: 'Enter' })
    expect(submits).toHaveLength(0)
    await input.trigger('keydown', { key: 'Enter', ctrlKey: true })
    expect(submits).toHaveLength(1)
    await input.trigger('keydown', { key: 'Enter', metaKey: true })
    expect(submits).toHaveLength(2)
  })

  test('日本語入力の変換中（isComposing）の Ctrl+Enter では送らない', async () => {
    const { w, submits } = mountForm('きげん')
    await w
      .get(tid('thread-ai-question-input'))
      .trigger('keydown', { key: 'Enter', ctrlKey: true, isComposing: true })
    expect(submits).toHaveLength(0)
  })

  test('空・空白だけ・生成中は送れず、ボタンも無効', async () => {
    for (const [text, disabled] of [
      ['', false],
      ['  \n ', false],
      ['質問', true],
    ] as const) {
      const { w, submits } = mountForm(text, disabled)
      expect(w.get(tid('thread-ai-ask')).attributes('disabled')).toBeDefined()
      await w
        .get(tid('thread-ai-question-input'))
        .trigger('keydown', { key: 'Enter', ctrlKey: true })
      expect(submits).toHaveLength(0)
    }
  })

  test('2,000 文字までの残りを出し、超えると送れない', async () => {
    const { w, submits } = mountForm('あ'.repeat(1990))
    expect(w.get(tid('thread-ai-question-remaining')).text()).toBe('残り 10 文字')
    await w.get(tid('thread-ai-ask')).trigger('submit')
    expect(submits).toHaveLength(1)

    await w.setProps({ modelValue: 'あ'.repeat(2001) })
    expect(w.get(tid('thread-ai-question-remaining')).classes()).toContain('over')
    expect(w.get(tid('thread-ai-ask')).attributes('disabled')).toBeDefined()
    await w.get(tid('thread-ai-question-input')).trigger('keydown', { key: 'Enter', ctrlKey: true })
    expect(submits).toHaveLength(1)
  })
})
