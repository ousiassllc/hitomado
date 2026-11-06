// Ousiass デザインガイド準拠の回帰テスト（#222）。タイポグラフィ（T5）の Mono の指定を確かめる。
import { describe, expect, test } from 'bun:test'
import { mount } from '@vue/test-utils'
import AIDestinationBox from '../../src/components/settings/ai/AIDestinationBox.vue'
import { rule } from './styleRule'

describe('桁を揃える文字の Mono（T5・#222）', () => {
  test('スレッド行のメッセージ件数（数字だけ）は --font-mono', () => {
    expect(rule('inbox/ThreadRow.vue', '.count {')).toContain('font-family: var(--font-mono)')
  })

  test('送信先の URL だけを別要素に分け、--font-mono にする', () => {
    const w = mount(AIDestinationBox, {
      props: {
        provider: 'openai',
        baseUrl: 'https://llm.example.com/v1',
        draftProvider: 'openai',
        draftBaseUrl: 'https://llm.example.com/v1',
      },
    })
    expect(w.get('[data-testid="ai-destination"] .url').text()).toBe('https://llm.example.com/v1')
    expect(rule('settings/ai/AIDestinationBox.vue', '.url {')).toContain('font-family: var(--font-mono)')
  })
})
