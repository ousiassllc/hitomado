import { afterAll, beforeEach, describe, expect, mock, test } from 'bun:test'
import { flushPromises, mount } from '@vue/test-utils'
import * as realAgent from '../../bindings/github.com/ousiassllc/hitomado/internal/app/agentcore/agentservice'
import * as realEvents from '../lib/events'

// 画面の端のマスコット（#372）: 件数のバッジ・押すとパネルを開く・✕で消す・イベントで件数が変わる。

type Handler = (payload: unknown) => void

// モックはプロセス全体で共有される。afterAll で元へ戻せるよう、本物のエクスポートをモックの前に控えておく。
const real = { agent: { ...realAgent }, events: { ...realEvents } }

const AGENT_UPDATED = 'agent:updated'
const handlers = new Map<string, Handler>()
const calls: string[] = []
const ctl = {
  list: async (): Promise<{ pendingCount: number; items: [] }> => ({ pendingCount: 0, items: [] }),
  open: async (): Promise<void> => undefined,
  setMascot: async (_enabled: boolean): Promise<unknown> => ({}),
}

mock.module(
  '../../bindings/github.com/ousiassllc/hitomado/internal/app/agentcore/agentservice',
  () => ({
    ...real.agent,
    ListAgentItems: () => ctl.list(),
    OpenAgentPanel: () => {
      calls.push('open')
      return ctl.open()
    },
    SetAgentMascot: (enabled: boolean) => {
      calls.push(`setMascot:${enabled}`)
      return ctl.setMascot(enabled)
    },
  }),
)
mock.module('../lib/events', () => ({
  ...real.events,
  onEvent: (name: string, h: Handler) => {
    handlers.set(name, h)
    return () => handlers.delete(name)
  },
}))

// モックを張ったあとに読み込む（静的 import は巻き上げられ、本物のモジュールを掴んでしまう）。
const { default: MascotApp } = await import('./MascotApp.vue')

afterAll(() => {
  mock.module(
    '../../bindings/github.com/ousiassllc/hitomado/internal/app/agentcore/agentservice',
    () => ({
      ...real.agent,
    }),
  )
  mock.module('../lib/events', () => ({ ...real.events }))
})

beforeEach(() => {
  handlers.clear()
  calls.length = 0
  ctl.list = async () => ({ pendingCount: 0, items: [] })
  ctl.open = async () => undefined
  ctl.setMascot = async () => ({})
})

async function mountMascot() {
  const w = mount(MascotApp)
  await flushPromises()
  return w
}

function emitUpdated(pendingCount: number): void {
  const h = handlers.get(AGENT_UPDATED)
  if (!h) throw new Error('agent:updated is not subscribed')
  h({ pendingCount, enabled: true, mascotEnabled: true })
}

describe('MascotApp', () => {
  test('件数が 0 でもマスコットは出し、バッジは出さない', async () => {
    const w = await mountMascot()
    expect(w.get('[data-testid=mascot-open]').attributes('aria-label')).toBe('エージェントを開く')
    expect(w.find('.badge').text()).toBe('')
    w.unmount()
  })

  test('起動時に一覧から件数を読み、バッジと名前に出す', async () => {
    ctl.list = async () => ({ pendingCount: 3, items: [] })
    const w = await mountMascot()
    expect(w.get('[data-testid=mascot-open]').attributes('aria-label')).toBe(
      'エージェントを開く・確認待ち 3 件',
    )
    expect(w.get('.badge').text()).toBe('3')
    w.unmount()
  })

  test('agent:updated で件数が変わり、0 になるとバッジが消える', async () => {
    const w = await mountMascot()
    emitUpdated(5)
    await flushPromises()
    expect(w.get('.badge').text()).toBe('5')
    expect(w.get('[data-testid=mascot-open]').attributes('aria-label')).toBe(
      'エージェントを開く・確認待ち 5 件',
    )

    emitUpdated(0)
    await flushPromises()
    expect(w.find('.badge').text()).toBe('')
    expect(w.get('[data-testid=mascot-open]').attributes('aria-label')).toBe('エージェントを開く')
    w.unmount()
  })

  test('読み込みの最中に届いたイベントの件数を、遅れて返った一覧の古い件数で上書きしない', async () => {
    const list = Promise.withResolvers<{ pendingCount: number; items: [] }>()
    ctl.list = () => list.promise
    const w = mount(MascotApp)
    emitUpdated(2)
    list.resolve({ pendingCount: 9, items: [] })
    await flushPromises()
    expect(w.get('.badge').text()).toBe('2')
    w.unmount()
  })

  test('一覧の読み込みに失敗しても、マスコットは件数 0 で出る', async () => {
    ctl.list = async () => {
      throw new Error('boom')
    }
    const w = await mountMascot()
    expect(w.get('[data-testid=mascot-open]').attributes('aria-label')).toBe('エージェントを開く')
    w.unmount()
  })

  test('押すと OpenAgentPanel を呼ぶ（失敗しても未処理の reject を出さない）', async () => {
    ctl.open = async () => {
      throw new Error('boom')
    }
    const w = await mountMascot()
    await w.get('[data-testid=mascot-open]').trigger('click')
    await flushPromises()
    expect(calls).toEqual(['open'])
    w.unmount()
  })

  test('✕ で SetAgentMascot(false) を呼ぶだけで、パネルは開かない', async () => {
    const w = await mountMascot()
    await w.get('[data-testid=mascot-hide]').trigger('click')
    await flushPromises()
    expect(calls).toEqual(['setMascot:false'])
    expect(w.get('[data-testid=mascot-hide]').attributes('aria-label')).toBe('マスコットを消す')
    w.unmount()
  })

  test('押す・消すはどちらも button（キーボードの Enter・Space で操作できる）で、✕ は押す操作の中に入れ子にしない', async () => {
    const w = await mountMascot()
    const open = w.get('[data-testid=mascot-open]')
    const hide = w.get('[data-testid=mascot-hide]')
    expect(open.element.tagName).toBe('BUTTON')
    expect(hide.element.tagName).toBe('BUTTON')
    expect(open.element.contains(hide.element)).toBe(false)
    w.unmount()
  })

  test('破棄するとイベントの購読を解除する', async () => {
    const w = await mountMascot()
    expect(handlers.has(AGENT_UPDATED)).toBe(true)
    w.unmount()
    expect(handlers.has(AGENT_UPDATED)).toBe(false)
  })
})
