import { afterAll, beforeEach, describe, expect, test } from 'bun:test'
import { mount } from '@vue/test-utils'
import {
  agentItem,
  agentOverview,
  installMocks,
  resetKit,
  responses,
  restoreMocks,
} from '../../../test/inboxKit'

installMocks()
afterAll(restoreMocks)

const { useAgentStore } = await import('../../stores/agent')
const { default: BottomTabs } = await import('./BottomTabs.vue')
const { default: SidebarAgent } = await import('./SidebarAgent.vue')

beforeEach(resetKit)

async function load(enabled: boolean, items: ReturnType<typeof agentItem>[] = []) {
  responses.GetAgentSettings = { enabled, mascotEnabled: false, aiReady: true }
  responses.ListAgentItems = agentOverview(items)
  await useAgentStore().load()
}

describe('サイドバーのエージェントの入口', () => {
  test.each([
    ['オフ', false, []],
    ['オン', true, []],
    ['オン', true, [agentItem('i1', { status: 'later' })]],
    ['確認待ち 2件', true, [agentItem('i1'), agentItem('i2', { status: 'asking' })]],
    ['オフ', false, [agentItem('i1')]],
  ] as const)('一言は「%s」', async (sub, enabled, items) => {
    await load(enabled, [...items])
    const w = mount(SidebarAgent)
    expect(w.get('.sub').text()).toBe(sub)
  })
})

describe('スマホの下タブのエージェント', () => {
  test('確認待ち・質問待ちの件数をバッジと読み上げに出し、0 件・オフでは出さない', async () => {
    await load(true, [
      agentItem('i1'),
      agentItem('i2', { status: 'asking' }),
      agentItem('i3', { status: 'later' }),
    ])
    const w = mount(BottomTabs)
    const tab = w.get('[data-testid=tab-agent]')
    expect(tab.get('.badge').text()).toBe('2')
    expect(tab.attributes('aria-label')).toBe('エージェント（確認待ち 2件）')

    await load(true, [agentItem('i3', { status: 'later' })])
    expect(tab.find('.badge').exists()).toBe(false)
    expect(tab.attributes('aria-label')).toBeUndefined()

    await load(false, [agentItem('i1')])
    expect(tab.find('.badge').exists()).toBe(false)
  })
})
