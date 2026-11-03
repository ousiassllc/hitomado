// 狭幅（1 画面ずつ）の画面遷移: 一覧 → スレッド → 返信 → 戻る。幅を変えても選択・下書きが残ること。
import { afterAll, afterEach, beforeEach, describe, expect, test } from 'bun:test'
import { createPinia, setActivePinia } from 'pinia'
import { effectScope } from 'vue'
import { BACK_HANDLER_NAME, installBackHandler } from '../../lib/androidBack'
import { BREAKPOINT_MEDIUM, BREAKPOINT_WIDE } from '../../lib/layout'
import {
  h,
  installMocks,
  openReply,
  reset,
  restoreMocks,
  useAccountsStore,
  useComposeStore,
  useInboxStore,
  useUiStore,
} from './composeHarness'

installMocks()
afterAll(restoreMocks)

const { useNavigation } = await import('../../composables/useNavigation')
const { useLayoutStore } = await import('../layout')
const { usePlatformStore } = await import('../platform')

const flushPromises = () => new Promise<void>((r) => setTimeout(r, 0))

const REF = { accountId: 'a1', threadId: 't' }
const OTHER = { accountId: 'a1', threadId: 'other' }

/** matchMedia の代わり。幅（px）を変えると、登録された change リスナーを呼ぶ。 */
function fakeViewport(initial: number) {
  let width = initial
  const lists: { query: string; listeners: Set<() => void>; matches: boolean }[] = []
  const original = window.matchMedia
  window.matchMedia = ((query: string) => {
    const min = Number(/min-width: (\d+)px/.exec(query)?.[1])
    const list = {
      query,
      listeners: new Set<() => void>(),
      get matches() {
        return width >= min
      },
      addEventListener: (_: string, l: () => void) => list.listeners.add(l),
      removeEventListener: (_: string, l: () => void) => list.listeners.delete(l),
    }
    lists.push(list)
    return list
  }) as unknown as typeof window.matchMedia
  return {
    resize(w: number) {
      width = w
      for (const l of lists) for (const f of l.listeners) f()
    },
    restore() {
      window.matchMedia = original
    },
  }
}

let viewport: ReturnType<typeof fakeViewport>

function setup(touchUi: boolean, width: number) {
  setActivePinia(createPinia())
  const accounts = useAccountsStore()
  accounts.accounts = [
    { id: 'a1', email: 'a1@example.com', displayName: 'A', color: 'red', signatureHtml: '' },
  ] as typeof accounts.accounts
  const platform = usePlatformStore()
  platform.capabilities = { ...platform.capabilities, touchUi }
  viewport = fakeViewport(width)
  const layout = useLayoutStore()
  layout.start()
  const scope = effectScope()
  const nav = scope.run(() => useNavigation()) as ReturnType<typeof useNavigation>
  return { layout, nav, inbox: useInboxStore(), compose: useComposeStore(), ui: useUiStore() }
}

beforeEach(reset)
afterEach(() => {
  useLayoutStore().dispose()
  viewport.restore()
  document.documentElement.removeAttribute('data-touch-ui')
  delete document.documentElement.dataset.layout
})

describe('1 画面ずつの遷移（狭幅）', () => {
  test('一覧 → スレッド → 返信 と進み、戻るで 返信 → スレッド → 一覧 と戻り、一覧では戻る先なし', async () => {
    const { layout, nav, inbox, compose } = setup(true, 390)
    expect(layout.mode).toBe('stack')
    expect(nav.screen.value).toBe('list')

    inbox.select(REF)
    expect(nav.screen.value).toBe('thread')

    await openReply(compose)
    expect(nav.screen.value).toBe('reply')

    expect(nav.goBack()).toBe(true) // 返信 → スレッド
    await flushPromises()
    expect(compose.session).toBeUndefined()
    expect(nav.screen.value).toBe('thread')
    expect(inbox.selectedKey).toBeDefined()

    expect(nav.goBack()).toBe(true) // スレッド → 一覧
    expect(nav.screen.value).toBe('list')
    expect(inbox.selectedKey).toBeUndefined()

    expect(nav.goBack()).toBe(false) // 一覧（ルート）: アプリ側でバックグラウンドへ
  })

  test('新規作成は作成画面。戻るで閉じる', async () => {
    const { nav, compose } = setup(true, 390)
    h.responses.OpenCompose = {
      sessionId: 's2',
      mode: 'new',
      accountId: 'a1',
      senderLocked: false,
      content: { to: [], cc: null, bcc: null, subject: '', bodyHtml: '' },
      attachments: null,
    }
    await compose.open({ mode: 'new' })
    expect(nav.screen.value).toBe('compose')
    expect(nav.goBack()).toBe(true)
    await flushPromises()
    expect(nav.screen.value).toBe('list')
  })

  test('別のスレッドを開くと、前のスレッドの下書きは「返信画面」ではなく裏に残る', async () => {
    const { nav, inbox, compose } = setup(true, 390)
    inbox.select(REF)
    await openReply(compose)
    inbox.select(OTHER)
    expect(nav.screen.value).toBe('thread')
    expect(compose.session).toBeDefined()
    inbox.select(REF)
    expect(nav.screen.value).toBe('reply')
  })

  test('ダイアログ（設定）が開いているときの戻るは、画面を戻さずダイアログを閉じる（Esc を送る）', () => {
    const { nav, inbox, ui } = setup(true, 390)
    inbox.select(REF)
    ui.openDialog('settings')
    let escapes = 0
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') escapes++
    }
    document.addEventListener('keydown', onKey)
    expect(nav.goBack()).toBe(true)
    document.removeEventListener('keydown', onKey)
    expect(escapes).toBe(1)
    expect(nav.screen.value).toBe('thread')
  })

  test('ドロワー（サイドバー）が開いているときの戻るは、ドロワーを閉じる', () => {
    const { nav, inbox, ui } = setup(true, 390)
    inbox.select(REF)
    ui.openMenu()
    expect(nav.goBack()).toBe(true)
    expect(ui.menuOpen).toBe(false)
    expect(nav.screen.value).toBe('thread')
  })

  test('検索結果の一覧では、戻るで受信箱へ戻る', async () => {
    const { nav, inbox } = setup(true, 390)
    h.responses.ListInbox = { items: [], failures: [] }
    inbox.mode = 'search' as 'inbox' | 'search'
    expect(nav.goBack()).toBe(true)
    await flushPromises()
    expect(inbox.mode as string).toBe('inbox')
  })
})

describe('幅の切り替え', () => {
  test('ブレイクポイントで 3 ペイン／2 ペイン／1 画面ずつが切り替わり、html の属性にも反映される', () => {
    const { layout } = setup(true, 1200)
    expect(layout.mode).toBe('wide')
    expect(layout.hasDrawer).toBe(false)
    viewport.resize(BREAKPOINT_WIDE - 1)
    expect(layout.mode).toBe('medium')
    expect(layout.hasDrawer).toBe(true)
    viewport.resize(BREAKPOINT_MEDIUM - 1)
    expect(layout.mode).toBe('stack')
    expect(layout.isStack).toBe(true)
    expect(document.documentElement.dataset.layout).toBe('stack')
    expect(document.documentElement.hasAttribute('data-touch-ui')).toBe(true)
  })

  test('幅を変えても、開いているスレッドと返信の下書きは消えない', async () => {
    const { layout, nav, inbox, compose } = setup(true, 1200)
    inbox.select(REF)
    await openReply(compose)
    compose.content.subject = '編集中の件名'

    viewport.resize(390)
    expect(layout.mode).toBe('stack')
    expect(inbox.selectedKey).toBeDefined()
    expect(compose.session?.sessionId).toBe('s1')
    expect(compose.content.subject).toBe('編集中の件名')
    expect(nav.screen.value).toBe('reply')

    viewport.resize(700)
    expect(layout.mode).toBe('medium')
    viewport.resize(1200)
    expect(layout.mode).toBe('wide')
    expect(inbox.selectedKey).toBeDefined()
    expect(compose.content.subject).toBe('編集中の件名')
  })

  test('3 ペインに戻ると、開いていたドロワーは閉じる', () => {
    const { ui } = setup(true, 700)
    ui.openMenu()
    viewport.resize(1200)
    expect(ui.menuOpen).toBe(false)
  })

  test('タッチ UI でない OS（Windows）は、幅が狭くても 3 ペインのまま・タッチ用の属性も付かない', () => {
    const { layout } = setup(false, 390)
    expect(layout.mode).toBe('wide')
    expect(document.documentElement.hasAttribute('data-touch-ui')).toBe(false)
  })

  test('戻るの意味は 3 ペインでも同じ（スレッド → 一覧）。ただし受け口を入れるのはタッチ UI の OS だけ（AppShell）', () => {
    const { nav, inbox } = setup(false, 1200)
    inbox.select(REF)
    expect(nav.goBack()).toBe(true)
    expect(inbox.selectedKey).toBeUndefined()
  })
})

describe('Android の戻るボタンの受け口', () => {
  test('MainActivity.java が呼ぶ window.hitomadoHandleBack に登録され、解除できる', () => {
    const w = window as unknown as Record<string, unknown>
    const off = installBackHandler(() => true)
    expect(typeof w[BACK_HANDLER_NAME]).toBe('function')
    expect((w[BACK_HANDLER_NAME] as () => boolean)()).toBe(true)
    off()
    expect(w[BACK_HANDLER_NAME]).toBeUndefined()
  })

  test('別のハンドラに置き換えられていたら、古い解除は新しいハンドラを消さない', () => {
    const w = window as unknown as Record<string, unknown>
    const off1 = installBackHandler(() => true)
    const off2 = installBackHandler(() => false)
    off1()
    expect(typeof w[BACK_HANDLER_NAME]).toBe('function')
    off2()
    expect(w[BACK_HANDLER_NAME]).toBeUndefined()
  })
})
