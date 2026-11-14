import { describe, expect, test } from 'bun:test'
import { features } from '../../../../../test/aiConfigKit'
import { acctA, base, chatgptCfg } from '../aiTestSupport'
import { featureChoices, optionLabel, optionValue, parseOption } from './aiFeatures'

const models = {
  openai: { provider: 'openai', models: [{ id: 'gpt-a' }, { id: 'gpt-b', displayName: 'GPT B' }] },
  chatgpt: { provider: 'chatgpt', models: [{ id: 'gpt-plan' }] },
  anthropic: { provider: 'anthropic', models: [{ id: 'claude-x' }] },
}
const none = { provider: '', model: '' }
const connected = {
  ...base,
  ...chatgptCfg([acctA]),
  apiKeyProviders: ['openai', 'anthropic', 'typesafe'],
  consentedProviders: ['openai', 'chatgpt', 'typesafe'],
}

describe('機能ごとの選択肢（#376）', () => {
  test('値は「プロバイダ|モデル」。表示は「サービス名 ・ モデル」で、Jev はサービス名だけ', () => {
    expect(parseOption(optionValue('openai', 'gpt-a'))).toEqual({
      provider: 'openai',
      model: 'gpt-a',
    })
    expect(parseOption(optionValue('typesafe', ''))).toEqual({ provider: 'typesafe', model: '' })
    expect(optionLabel('openai', 'gpt-b', 'GPT B')).toBe('OpenAI 互換 ・ GPT B（gpt-b）')
    expect(optionLabel('typesafe', '')).toBe('TypeSafe AI（Jev）')
  })

  test('つないであるプロバイダのモデルだけが並ぶ（接続済みでない Anthropic は出さない。同意前の AI も出さない）', () => {
    const c = featureChoices('draft', connected, models, none)
    expect(c.options.map((o) => o.value)).toEqual([
      'chatgpt|gpt-plan',
      'openai|gpt-a',
      'openai|gpt-b',
    ])
    expect(c.noneAvailable).toBe(false)
  })

  test('Jev は文章を書かないので「AI で下書き」には出ず、Inbox Zero の選択肢にだけ出る', () => {
    const draft = featureChoices('draft', connected, models, none).options.map((o) => o.value)
    const zero = featureChoices('inboxZero', connected, models, none).options.map((o) => o.value)
    expect(draft).not.toContain('typesafe|')
    expect(zero).toContain('typesafe|')
  })

  test('何もつないでいなければ、使える AI が無い。Jev だけなら「AI で下書き」は使える AI が無く、Inbox Zero は Jev だけ', () => {
    const empty = featureChoices(
      'draft',
      { ...base, apiKeyProviders: [], consentedProviders: [] },
      {},
      none,
    )
    expect(empty).toMatchObject({ options: [], noneAvailable: true })
    const jev = { ...base, apiKeyProviders: ['typesafe'], consentedProviders: ['typesafe'] }
    expect(featureChoices('draft', jev, {}, none).noneAvailable).toBe(true)
    const zero = featureChoices('inboxZero', jev, {}, none)
    expect(zero.options).toEqual([{ value: 'typesafe|', label: 'TypeSafe AI（Jev）' }])
  })

  test('選んでいた AI が外されても、選択は選択肢に残す（別の AI に置き換えない）', () => {
    const removed = {
      ...connected,
      apiKeyProviders: ['anthropic'],
      features: features({ provider: 'openai', model: 'gpt-a' }),
    }
    const c = featureChoices('draft', removed, models, removed.features.draft)
    expect(c.options.map((o) => o.value)).toEqual(['chatgpt|gpt-plan', 'openai|gpt-a'])
    expect(c.options.at(-1)?.label).toBe('OpenAI 互換 ・ gpt-a')
  })

  test('一覧に無いモデルを選んでいるときは「一覧にありません」を付けて残す', () => {
    const c = featureChoices('draft', connected, models, { provider: 'openai', model: 'gpt-old' })
    expect(c.options.at(-1)).toEqual({
      value: 'openai|gpt-old',
      label: 'OpenAI 互換 ・ gpt-old（一覧にありません）',
    })
  })

  test('一覧を取得できなかったプロバイダは手入力にする（理由つき）。まだ取得していなければ読み込み中', () => {
    const failed = {
      ...models,
      openai: {
        provider: 'openai',
        models: [],
        reason: 'unsupported',
        message: '取得機能がありません',
      },
    }
    const c = featureChoices('draft', connected, failed, none)
    expect(c.manual).toEqual([{ provider: 'openai', message: '取得機能がありません' }])
    expect(c.options.map((o) => o.value)).toEqual(['chatgpt|gpt-plan'])
    expect(featureChoices('draft', connected, { chatgpt: models.chatgpt }, none).loading).toBe(true)
  })
})
