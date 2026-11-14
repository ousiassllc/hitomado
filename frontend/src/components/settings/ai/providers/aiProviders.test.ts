import { beforeEach, describe, expect, test } from 'bun:test'
import { features } from '../../../../../test/aiConfigKit'
import { acctA, base, chatgptCfg } from '../aiTestSupport'
import { connectionState, isConnected, needsConsent } from './aiProviders'

beforeEach(() => {
  document.body.innerHTML = ''
})

const cfg = (c: Partial<typeof base> = {}) => ({ ...base, ...c })

describe('つなぎ先の状態（#376）', () => {
  test('資格情報があり送信に同意していれば「接続済み」。ChatGPT はログイン、ほかは API キー', () => {
    expect(isConnected('openai', cfg())).toBe(true)
    expect(connectionState('openai', cfg())).toBe('connected')
    const gpt = cfg({ ...chatgptCfg([acctA]), consentedProviders: ['chatgpt'] })
    expect(connectionState('chatgpt', gpt)).toBe('connected')
    expect(connectionState('anthropic', cfg())).toBe('disconnected')
  })

  test('資格情報だけで同意がまだなら接続済みにしない（同意待ち）', () => {
    const c = cfg({ consentedProviders: [] })
    expect(isConnected('openai', c)).toBe(false)
    expect(needsConsent('openai', c)).toBe(true)
    expect(connectionState('openai', c)).toBe('reconnect') // 機能で選んでいるのに使えない
    expect(needsConsent('anthropic', c)).toBe(false)
  })

  test('機能で選んでいるのにつながっていない・ChatGPT がログアウトした: 「つなぎ直しが必要」。選んでいないなら「未接続」', () => {
    const removed = cfg({ apiKeyProviders: [] }) // draft は openai を選んだまま
    expect(connectionState('openai', removed)).toBe('reconnect')
    expect(connectionState('anthropic', removed)).toBe('disconnected')
    const jevSelected = cfg({ features: features('', 'typesafe') })
    expect(connectionState('typesafe', jevSelected)).toBe('reconnect')
    const loggedOut = cfg({ ...chatgptCfg([{ ...acctA, signedIn: false }]), features: features() })
    expect(connectionState('chatgpt', loggedOut)).toBe('reconnect')
    expect(connectionState('chatgpt', cfg({ features: features() }))).toBe('disconnected')
  })
})
