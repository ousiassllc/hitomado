import { afterAll, afterEach, beforeEach, describe, expect, test } from 'bun:test'
import { mount, type VueWrapper } from '@vue/test-utils'
import { defineComponent, h } from 'vue'
import { callsOf, installMocks, ref, resetKit, restoreMocks, until } from '../inboxKit'

installMocks()
afterAll(restoreMocks)

const { useShortcuts } = await import('../../src/composables/useShortcuts')
const { useInboxStore } = await import('../../src/stores/inbox')
const { useDraftsStore } = await import('../../src/stores/drafts')

const mounted: VueWrapper[] = []
afterEach(() => {
  for (const w of mounted.splice(0)) w.unmount()
})
beforeEach(() => {
  resetKit()
  document.body.innerHTML = ''
})

const Host = defineComponent({
  setup() {
    useShortcuts()
    return () => h('div')
  },
})

function press(key: string, init: KeyboardEventInit = {}): void {
  window.dispatchEvent(
    new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true, ...init }),
  )
}

// 下書きの一覧（#366）を出している間は、見えていない受信箱の行を対象にした操作が動かない。見えていない行を黙ってアーカイブ・削除しないため。
describe('下書きの一覧を出している間のショートカット', () => {
  async function setup() {
    const inbox = useInboxStore()
    const drafts = useDraftsStore()
    await inbox.load()
    mounted.push(mount(Host))
    return { inbox, drafts }
  }

  test('j / k / o・アーカイブ・削除・既読・未読は、受信箱の行に何もしない', async () => {
    const { inbox, drafts } = await setup()
    inbox.toggleChecked(ref('a1', 't1'))
    drafts.active = true
    for (const [key, init] of [
      ['j', {}],
      ['k', {}],
      ['o', {}],
      ['e', {}],
      ['#', { shiftKey: true }],
      ['I', { shiftKey: true }],
      ['U', { shiftKey: true }],
    ] as const) {
      press(key, init)
    }
    await new Promise((r) => setTimeout(r, 0))
    expect(inbox.selectedKey).toBeUndefined()
    for (const name of ['Archive', 'Trash', 'MarkRead']) expect(callsOf(name)).toHaveLength(0)
  })

  test('返信の下書きを開いてスレッドが見えているときは、そのスレッドへのアーカイブ・既読は動く。j / k は動かない', async () => {
    const { inbox, drafts } = await setup()
    inbox.select(ref('a1', 't1'))
    drafts.active = true
    press('j')
    expect(inbox.selectedKey).toBe('a1:t1')
    press('I', { shiftKey: true })
    await until(() => callsOf('MarkRead').length === 1)
    expect(callsOf('MarkRead')[0].args[0]).toEqual([ref('a1', 't1')])
    press('e')
    await until(() => callsOf('Archive').length === 1)
    expect(callsOf('Archive')[0].args[0]).toEqual([ref('a1', 't1')])
  })

  test('一覧を出していなければ、同じキーが動く（受信箱の従来の操作は変わらない）', async () => {
    const { inbox } = await setup()
    inbox.toggleChecked(ref('a1', 't1'))
    press('e')
    await until(() => callsOf('Archive').length === 1)
  })

  test('g → i は下書きの一覧を閉じて受信箱へ戻る', async () => {
    const { drafts } = await setup()
    drafts.active = true
    press('g')
    press('i')
    expect(drafts.active).toBe(false)
  })
})
