import { beforeEach, describe, expect, test } from 'bun:test'
import { mount } from '@vue/test-utils'
import type { AIModelList } from '../../../../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import { COMMON_EFFORTS, effortChoices } from '../../../../lib/aiConfig'
import BaseSelect from '../../../ui/BaseSelect.vue'
import AIEffortField from '../AIEffortField.vue'
import AIModelField from '../AIModelField.vue'
import { base, button, flushPromises, mountAI, setup } from '../aiTestSupport'

beforeEach(() => {
  document.body.innerHTML = ''
})

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

const effortEl = (w: ReturnType<typeof mountAI>) => w.find('[data-testid="ai-effort"]')
const effortSelect = (w: ReturnType<typeof mountAI>) =>
  w.getComponent(AIEffortField).getComponent(BaseSelect)
const optionValues = (w: ReturnType<typeof mountAI>) =>
  (effortSelect(w).props('options') as { value: string }[]).map((o) => o.value)

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

describe('AISettings のエフォート（#111）', () => {
  test('OpenAI 互換・ChatGPT プランは共通の 3 段階を出す', async () => {
    const openai = setup({ provider: 'openai', effort: 'medium' })
    const w = mountAI(openai.pinia)
    await flushPromises()
    expect(optionValues(w).slice(1)).toEqual(['low', 'medium', 'high'])
    expect(effortSelect(w).props('modelValue')).toBe('medium')
    w.unmount()

    document.body.innerHTML = ''
    const gpt = setup({ provider: 'chatgpt', model: 'gpt-5' })
    const w2 = mountAI(gpt.pinia)
    expect(optionValues(w2).slice(1)).toEqual(['low', 'medium', 'high'])
    expect(effortSelect(w2).props('modelValue')).toBe('default')
    w2.unmount()
  })

  test('Anthropic は選んだモデルの段階だけを出し、非対応のモデル・手入力のモデルでは選択を出さない', async () => {
    const { pinia } = setup(
      { provider: 'anthropic', model: 'claude-mid' },
      { anthropic: anthropicModels },
    )
    const w = mountAI(pinia)
    expect(optionValues(w).slice(1)).toEqual(['low', 'high'])

    const modelSelect = w.getComponent(AIModelField).getComponent(BaseSelect)
    modelSelect.vm.$emit('update:modelValue', 'claude-new')
    await flushPromises()
    expect(optionValues(w).slice(1)).toEqual(ALL)

    modelSelect.vm.$emit('update:modelValue', 'claude-old')
    await flushPromises()
    expect(effortEl(w).exists()).toBe(false)
    w.unmount()

    document.body.innerHTML = ''
    const manual = setup({ provider: 'anthropic', model: 'typed' }) // 一覧が取れず手入力
    const w2 = mountAI(manual.pinia)
    expect(effortEl(w2).exists()).toBe(false)
    w2.unmount()
  })

  test('選んだエフォートは、設定を保存するときに渡す。変えると保存できる状態になる', async () => {
    const { pinia, store } = setup({ provider: 'openai', model: 'gpt-old' })
    const saved: unknown[][] = []
    store.setAIConfig = async (...a: unknown[]) => {
      saved.push(a)
    }
    const w = mountAI(pinia)
    expect(button(w, '設定を保存').attributes('disabled')).toBeDefined()
    effortSelect(w).vm.$emit('update:modelValue', 'high')
    await flushPromises()
    expect(button(w, '設定を保存').attributes('disabled')).toBeUndefined()
    await button(w, '設定を保存').trigger('click')
    await flushPromises()
    expect(saved).toEqual([['openai', 'gpt-old', '', 'high']])
    w.unmount()
  })

  test('「モデルの既定」を選ぶと、空文字を渡す（保存済みのエフォートを外せる）', async () => {
    const { pinia, store } = setup({ provider: 'openai', model: 'gpt-old', effort: 'low' })
    const saved: unknown[][] = []
    store.setAIConfig = async (...a: unknown[]) => {
      saved.push(a)
    }
    const w = mountAI(pinia)
    const first = (effortSelect(w).props('options') as { value: string }[])[0]?.value
    effortSelect(w).vm.$emit('update:modelValue', first)
    await flushPromises()
    await button(w, '設定を保存').trigger('click')
    await flushPromises()
    expect(saved).toEqual([['openai', 'gpt-old', '', '']])
    w.unmount()
  })

  test('モデルを変えて選択中の段階が使えなくなったら「モデルの既定」に戻る。使えるなら引き継ぐ', async () => {
    const { pinia } = setup(
      { provider: 'anthropic', model: 'claude-new', effort: 'max' },
      { anthropic: anthropicModels },
    )
    const w = mountAI(pinia)
    expect(effortSelect(w).props('modelValue')).toBe('max')
    const modelSelect = w.getComponent(AIModelField).getComponent(BaseSelect)

    modelSelect.vm.$emit('update:modelValue', 'claude-mid') // max は使えない
    await flushPromises()
    expect(effortSelect(w).props('modelValue')).toBe('default')

    effortSelect(w).vm.$emit('update:modelValue', 'high')
    modelSelect.vm.$emit('update:modelValue', 'claude-new') // high は使える
    await flushPromises()
    expect(effortSelect(w).props('modelValue')).toBe('high')
    w.unmount()
  })

  test('プロバイダを変えて使えなくなったら「モデルの既定」に戻る（xhigh は OpenAI 互換の段階にない）', async () => {
    const { pinia } = setup(
      { provider: 'anthropic', model: 'claude-new', effort: 'xhigh' },
      { anthropic: anthropicModels },
    )
    const w = mountAI(pinia)
    const provider = w
      .findAllComponents(BaseSelect)
      .find((s) => s.props('ariaLabel') === 'プロバイダ')
    provider?.vm.$emit('update:modelValue', 'openai')
    await flushPromises()
    expect(effortSelect(w).props('modelValue')).toBe('default')
    w.unmount()
  })

  test('保存済みのエフォートは、一覧がまだ取れていなくても消さない（選択肢が出ないだけで、設定は変わらない）', () => {
    const { pinia } = setup({ ...base, provider: 'anthropic', model: 'claude-new', effort: 'high' })
    const w = mountAI(pinia)
    expect(effortEl(w).exists()).toBe(false)
    expect(button(w, '設定を保存').attributes('disabled')).toBeDefined() // 変更なし = 保存済みのまま
    w.unmount()
  })
})
