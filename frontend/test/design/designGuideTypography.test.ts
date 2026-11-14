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
    expect(rule('settings/ai/AIDestinationBox.vue', '.url {')).toContain(
      'font-family: var(--font-mono)',
    )
  })
})

describe('行間はスケールの値（T3・T4・#227）', () => {
  test('プレーンテキスト本文は 16px・1.8（sandbox iframe の逸脱とは別）', () => {
    const body = rule('thread/MessageBody.vue', '.body-text {')
    expect(body).toContain('font-size: var(--font-size-base)')
    expect(body).toContain('line-height: var(--line-height-base)')
  })

  test.each([['ui/CountBadge.vue', '.badge {']])(
    '件数バッジ %s %s は line-height に xs を持ち、min-height 20px を超えない',
    (file, selector) => {
      const badge = rule(file, selector)
      expect(badge).toContain('line-height: var(--line-height-xs)')
      expect(badge).toContain('min-height: 20px')
    },
  )

  test.each([
    ['ui/BaseTooltip.vue', ':global(.tip-pop) {', 'xs'],
    ['thread/ThreadHeader.vue', '.meta-row {', 'sm'],
    ['compose/AttachmentBar.vue', '.attach-hint {', 'xs'],
    ['compose/ComposePane.vue', '.foot-hint {', 'xs'],
    ['settings/ai/ChatGPTAccountRow.vue', '.pick {', 'sm'],
  ])('折り返しうる %s %s は font-size と対の line-height を持つ', (file, selector, size) => {
    const r = rule(file, selector)
    expect(r).toContain(`font-size: var(--font-size-${size})`)
    expect(r).toContain(`line-height: var(--line-height-${size})`)
  })

  test.each([
    ['settings/McpSettings.vue', '.state {'],
    ['settings/accounts/AccountRow.vue', '.status {'],
    ['settings/oauth/OAuthClientsSettings.vue', '.inuse {'],
    ['settings/SignatureSettings.vue', '.dirty {'],
    ['settings/ai/ChatGPTAccountRow.vue', '.badge {'],
  ])('1 行の状態ピル %s %s は font-size xs と対の line-height を持つ（#231）', (file, selector) => {
    const r = rule(file, selector)
    expect(r).toContain('font-size: var(--font-size-xs)')
    expect(r).toContain('line-height: var(--line-height-xs)')
  })
})
