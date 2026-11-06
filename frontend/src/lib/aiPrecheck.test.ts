import { describe, expect, test } from 'bun:test'
import { type AiPrecheckConfig, aiPrecheck } from './aiPrecheck'

const base: AiPrecheckConfig = {
  provider: 'openai',
  configured: true,
  hasApiKey: true,
  consentedProviders: ['openai'],
}

describe('aiPrecheck（useAiDraft・useThreadAi 共通の前提確認。F-08）', () => {
  test('設定が未保存→キー未設定→未同意の順に、次に必要な確認を返す', () => {
    expect(
      aiPrecheck({ ...base, configured: false, hasApiKey: false, consentedProviders: [] }),
    ).toBe('setup')
    expect(aiPrecheck({ ...base, hasApiKey: false, consentedProviders: [] })).toBe('key')
    expect(aiPrecheck({ ...base, consentedProviders: [] })).toBe('consent')
    expect(aiPrecheck({ ...base, consentedProviders: null })).toBe('consent')
    expect(aiPrecheck(base)).toBe('ready')
  })

  test('同意は現在のプロバイダに対してだけ見る。設定の未ロード中はキー未設定として扱う（従来の useAiDraft と同じ）', () => {
    expect(aiPrecheck({ ...base, consentedProviders: ['anthropic'] })).toBe('consent')
    expect(aiPrecheck(undefined)).toBe('key')
  })
})
