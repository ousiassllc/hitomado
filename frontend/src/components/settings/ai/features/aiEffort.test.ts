import { describe, expect, test } from 'bun:test'
import { mount } from '@vue/test-utils'
import type { AIModelList } from '../../../../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import { COMMON_EFFORTS, effortChoices } from '../../../../lib/aiConfig'
import BaseSelect from '../../../ui/BaseSelect.vue'
import AIEffortField from '../AIEffortField.vue'

const ALL = ['low', 'medium', 'high', 'xhigh', 'max']

const anthropicModels: AIModelList = {
  provider: 'anthropic',
  models: [
    { id: 'claude-new', efforts: ALL },
    { id: 'claude-mid', efforts: ['low', 'high'] },
    { id: 'claude-old' },
    { id: 'claude-null', efforts: null },
  ],
}

describe('effortChoices（#111）', () => {
  test('anthropic は選んだモデルの efforts だけ。対応しない・一覧に無い・一覧が無いときは空', () => {
    expect(effortChoices('anthropic', 'claude-new', anthropicModels)).toEqual(ALL)
    expect(effortChoices('anthropic', ' claude-mid ', anthropicModels)).toEqual(['low', 'high'])
    expect(effortChoices('anthropic', 'claude-old', anthropicModels)).toEqual([])
    expect(effortChoices('anthropic', 'claude-null', anthropicModels)).toEqual([])
    expect(effortChoices('anthropic', 'typed-by-hand', anthropicModels)).toEqual([])
    expect(effortChoices('anthropic', 'claude-new', undefined)).toEqual([])
    expect(
      effortChoices('anthropic', 'claude-new', { provider: 'anthropic', models: null as never }),
    ).toEqual([])
  })

  test('openai・chatgpt は一覧に依らず共通の low／medium／high', () => {
    expect(COMMON_EFFORTS).toEqual(['low', 'medium', 'high'])
    for (const p of ['openai', 'chatgpt']) {
      expect(effortChoices(p, '', undefined)).toEqual(COMMON_EFFORTS)
      expect(
        effortChoices(p, 'anything', { provider: p, models: [{ id: 'anything', efforts: ALL }] }),
      ).toEqual(COMMON_EFFORTS)
    }
  })
})

describe('AIEffortField', () => {
  test('先頭は常に「モデルの既定」。段階が空なら選択自体を出さない', () => {
    const w = mount(AIEffortField, { props: { efforts: ['low', 'high'], modelValue: '' } })
    const opts = w.getComponent(BaseSelect).props('options') as { value: string; label: string }[]
    expect(opts[0]?.label).toBe('モデルの既定')
    expect(opts.slice(1).map((o) => o.value)).toEqual(['low', 'high'])
    expect(w.getComponent(BaseSelect).props('modelValue')).toBe(opts[0]?.value)

    const none = mount(AIEffortField, { props: { efforts: [], modelValue: '' } })
    expect(none.find('[data-testid="ai-effort"]').exists()).toBe(false)
  })

  test('選んだ段階は update:modelValue で返り、「モデルの既定」は空文字になる', async () => {
    const w = mount(AIEffortField, { props: { efforts: ['low', 'high'], modelValue: 'high' } })
    const sel = w.getComponent(BaseSelect)
    expect(sel.props('modelValue')).toBe('high')
    sel.vm.$emit('update:modelValue', 'low')
    sel.vm.$emit('update:modelValue', (sel.props('options') as { value: string }[])[0]?.value)
    expect(w.emitted('update:modelValue')).toEqual([['low'], ['']])
  })
})
