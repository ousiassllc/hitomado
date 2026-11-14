import { afterAll, afterEach, beforeEach, describe, expect, mock, test } from 'bun:test'
import { mount, type VueWrapper } from '@vue/test-utils'
import { defineComponent, h } from 'vue'
import { keyOf } from '../../src/lib/inboxItems'
import { callsOf, installMocks, ref, resetKit, restoreMocks, until } from '../inboxKit'

const key = (a: string, t: string) => keyOf(ref(a, t))

installMocks()
afterAll(restoreMocks)

const { useShortcuts } = await import('../../src/composables/useShortcuts')
const { useInboxStore } = await import('../../src/stores/inbox')
const { useComposeStore } = await import('../../src/stores/compose')
const { useUiStore } = await import('../../src/stores/ui')

// 失敗したテストのリスナーが後続へ残らないよう、テストごとに必ず外す
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

/** 一覧（a1/t1..a2/t4）を読み込み済みのストアと、ショートカットを登録したホストを用意する。 */
async function setup() {
  const inbox = useInboxStore()
  const compose = useComposeStore()
  const ui = useUiStore()
  const open = mock(async (_req: unknown) => true)
  compose.open = open
  await inbox.load()
  mounted.push(mount(Host))
  return { inbox, compose, ui, open }
}

/** keydown を発火する（target に渡した要素から bubbling する。既定は window）。 */
function press(
  key: string,
  init: KeyboardEventInit = {},
  target: EventTarget = window,
): KeyboardEvent {
  const ev = new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true, ...init })
  target.dispatchEvent(ev)
  return ev
}

describe('移動（j / k / o / Enter）', () => {
  test('スレッドを開いていないときは、カーソルだけ動いて開かない', async () => {
    const { inbox } = await setup()
    press('j')
    expect(inbox.cursorKey).toBe(key('a1', 't1'))
    press('j')
    expect(inbox.cursorKey).toBe(key('a1', 't2'))
    press('k')
    expect(inbox.cursorKey).toBe(key('a1', 't1'))
    expect(inbox.selectedKey).toBeUndefined()
  })

  test('スレッドを開いているときは、j / k で開くスレッドも切り替わる', async () => {
    const { inbox } = await setup()
    inbox.select(ref('a1', 't1'))
    press('j')
    expect(inbox.selectedKey).toBe(key('a1', 't2'))
    press('j')
    expect(inbox.selectedKey).toBe(key('a2', 't3'))
    press('k')
    expect(inbox.selectedKey).toBe(key('a1', 't2'))
  })

  test('o と Enter はカーソル行を開く。ボタン上の Enter は奪わない', async () => {
    const { inbox } = await setup()
    press('j')
    press('o')
    expect(inbox.selectedKey).toBe(key('a1', 't1'))
    inbox.select(undefined)
    press('Enter')
    expect(inbox.selectedKey).toBe(key('a1', 't1'))
    inbox.select(undefined)
    const btn = document.body.appendChild(document.createElement('button'))
    press('Enter', {}, btn)
    expect(inbox.selectedKey).toBeUndefined()
  })
})

describe('一覧の操作（e / # / Shift+I / Shift+U）', () => {
  test('カーソル行を対象に、対応するバインディングを呼ぶ', async () => {
    const { inbox } = await setup()
    press('j')
    press('I', { shiftKey: true })
    await until(() => callsOf('MarkRead').length === 1)
    expect(callsOf('MarkRead')[0].args).toEqual([[ref('a1', 't1')], true])
    press('U', { shiftKey: true })
    await until(() => callsOf('MarkRead').length === 2)
    expect(callsOf('MarkRead')[1].args).toEqual([[ref('a1', 't1')], false])
    press('#', { shiftKey: true })
    await until(() => callsOf('Trash').length === 1)
    expect(callsOf('Trash')[0].args[0]).toEqual([ref('a1', 't1')])
    press('e')
    await until(() => callsOf('Archive').length === 1)
    expect(inbox.items.length).toBeLessThan(4)
  })

  test('複数選択があれば、その全体を対象にする', async () => {
    const { inbox } = await setup()
    inbox.toggleChecked(ref('a1', 't1'))
    inbox.toggleChecked(ref('a2', 't3'))
    press('e')
    await until(() => callsOf('Archive').length === 1)
    expect(callsOf('Archive')[0].args[0]).toEqual([ref('a1', 't1'), ref('a2', 't3')])
  })

  test('キーの押しっぱなし（repeat）では状態を変える操作を繰り返さない', async () => {
    await setup()
    press('j')
    press('e', { repeat: true })
    press('#', { repeat: true, shiftKey: true })
    expect(callsOf('Archive')).toHaveLength(0)
    expect(callsOf('Trash')).toHaveLength(0)
  })
})

describe('返信・転送・新規作成（r / a / f / c）', () => {
  test('開いているスレッドを宛先に compose.open を呼ぶ', async () => {
    const { inbox, open } = await setup()
    inbox.select(ref('a2', 't3'))
    press('r')
    press('a')
    press('f')
    expect(open.mock.calls.map((c) => c[0])).toEqual([
      { mode: 'reply', ref: ref('a2', 't3') },
      { mode: 'reply_all', ref: ref('a2', 't3') },
      { mode: 'forward', ref: ref('a2', 't3') },
    ])
  })

  test('スレッドを開いていなければ返信系は何もしない。c は新規作成', async () => {
    const { open } = await setup()
    press('r')
    press('a')
    press('f')
    expect(open).not.toHaveBeenCalled()
    press('c')
    expect(open.mock.calls).toEqual([[{ mode: 'new' }]])
  })
})

describe('g → i と検索', () => {
  test('g の次の i で検索を解除して受信箱へ戻る。間に別のキーが入ると戻らない', async () => {
    const { inbox } = await setup()
    await inbox.runSearch('見積')
    expect(inbox.mode).toBe('search')
    press('g')
    press('j')
    press('i')
    expect(inbox.mode).toBe('search')
    press('g')
    const ev = press('i')
    expect(ev.defaultPrevented).toBe(true)
    await until(() => inbox.mode === 'inbox')
  })

  test('g → i は、送信済みなど受信箱以外の場所からも受信箱へ戻し、開いているエージェントのパネルも閉じる', async () => {
    const { inbox, ui } = await setup()
    await inbox.setView('sent')
    ui.openAgent()
    press('g')
    press('i')
    await until(() => inbox.view === 'inbox')
    expect(ui.agentOpen).toBe(false)
  })

  test('/ で検索窓（data-search-input）にフォーカスする', async () => {
    await setup()
    const input = document.body.appendChild(document.createElement('input'))
    input.setAttribute('data-search-input', '')
    press('/')
    expect(document.activeElement).toBe(input)
  })
})

describe('Esc（closeOrClear）', () => {
  test('ポップアップが開いていれば何も閉じない（Reka UI 側に任せる）', async () => {
    const { inbox, ui } = await setup()
    inbox.select(ref('a1', 't1'))
    ui.openAgent()
    const popup = document.body.appendChild(document.createElement('div'))
    popup.setAttribute('data-reka-popper-content-wrapper', '')
    press('Escape')
    expect(ui.agentOpen).toBe(true)
    expect(inbox.selectedKey).toBe(key('a1', 't1'))
  })

  test('エージェントのパネルが開いていれば、パネルだけ閉じる', async () => {
    const { inbox, ui } = await setup()
    inbox.select(ref('a1', 't1'))
    ui.openAgent()
    press('Escape')
    expect(ui.agentOpen).toBe(false)
    expect(inbox.selectedKey).toBe(key('a1', 't1'))
  })

  test('検索中は検索を解除する（選択の解除より優先）', async () => {
    const { inbox } = await setup()
    await inbox.runSearch('見積')
    inbox.select(ref('a1', 't1'))
    press('Escape')
    await until(() => inbox.mode === 'inbox')
  })

  test('検索中でなければ、開いているスレッドを閉じる', async () => {
    const { inbox } = await setup()
    inbox.select(ref('a1', 't1'))
    press('Escape')
    expect(inbox.selectedKey).toBeUndefined()
  })

  test('Esc は preventDefault しない。他のキーはする', async () => {
    await setup()
    expect(press('Escape').defaultPrevented).toBe(false)
    expect(press('j').defaultPrevented).toBe(true)
  })
})

describe('動かさない条件', () => {
  test('入力欄の中では何もしない', async () => {
    const { inbox, open } = await setup()
    inbox.select(ref('a1', 't1'))
    const input = document.body.appendChild(document.createElement('input'))
    const editor = document.body.appendChild(document.createElement('div'))
    editor.className = 'ProseMirror'
    for (const target of [input, editor]) {
      for (const key of ['j', 'e', 'r', 'c', 'Escape']) press(key, {}, target)
    }
    expect(inbox.cursorKey).toBe(key('a1', 't1'))
    expect(inbox.selectedKey).toBe(key('a1', 't1'))
    expect(callsOf('Archive')).toHaveLength(0)
    expect(open).not.toHaveBeenCalled()
  })

  test('ダイアログ表示中・修飾キー併用・IME 変換中・処理済みのイベントでは何もしない', async () => {
    const { ui, open } = await setup()
    press('c', { ctrlKey: true })
    press('c', { metaKey: true })
    press('c', { isComposing: true })
    const handled = new KeyboardEvent('keydown', { key: 'c', bubbles: true, cancelable: true })
    handled.preventDefault()
    window.dispatchEvent(handled)
    ui.openDialog('settings')
    press('c')
    expect(open).not.toHaveBeenCalled()
  })

  test('アンマウント後は何もしない', async () => {
    const { open } = await setup()
    mounted.pop()?.unmount()
    press('c')
    expect(open).not.toHaveBeenCalled()
  })
})
