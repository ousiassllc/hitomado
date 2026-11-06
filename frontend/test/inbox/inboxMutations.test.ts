import { afterAll, beforeEach, describe, expect, test } from 'bun:test'
import {
  appError,
  base,
  callsOf,
  deferred,
  failures,
  gates,
  gone,
  ids,
  installMocks,
  page,
  ref,
  resetKit,
  responses,
  restoreMocks,
  until,
} from '../inboxKit'

installMocks()
afterAll(restoreMocks)

const { useInboxStore } = await import('../../src/stores/inbox')
const { useUiStore } = await import('../../src/stores/ui')

beforeEach(resetKit)

describe('楽観的更新', () => {
  test('archive は呼び出し完了前にリストから消え、成功後も復活しない', async () => {
    const g = deferred()
    gates.Archive = g.p
    const s = useInboxStore()
    await s.load()
    const done = s.archive([ref('a1', 't1')])
    expect(ids(s)).toEqual(['t2', 't3', 't4'])
    g.resolve()
    expect(await done).toBe(true)
    expect(ids(s)).toEqual(['t2', 't3', 't4'])
    expect(callsOf('Archive')[0].args[0]).toEqual([ref('a1', 't1')])
  })

  test('複数選択は選択全体に作用する', async () => {
    const s = useInboxStore()
    await s.load()
    s.select(ref('a1', 't1'))
    s.toggleChecked(ref('a1', 't1'))
    s.toggleChecked(ref('a2', 't3'))
    expect(s.targetRefs).toEqual([ref('a1', 't1'), ref('a2', 't3')])
    await s.trash(s.targetRefs)
    expect(callsOf('Trash')[0].args[0]).toEqual([ref('a1', 't1'), ref('a2', 't3')])
    expect(ids(s)).toEqual(['t2', 't4'])
    expect(s.checkedKeys).toEqual([])
    expect(s.selectedKey).toBe('a1:t2') // 開いていた t1 の位置にきた次の行
  })
})

describe('アーカイブ・ゴミ箱後の次のスレッド', () => {
  test('開いているスレッドのアーカイブ後は次の行（中間）を開く', async () => {
    const s = useInboxStore()
    await s.load()
    s.select(ref('a1', 't2'))
    await s.archive([ref('a1', 't2')])
    expect(s.selectedKey).toBe('a2:t3')
    expect(s.cursorKey).toBe('a2:t3')
  })

  test('末尾の行なら一つ前を開く', async () => {
    const s = useInboxStore()
    await s.load()
    s.select(ref('a2', 't4'))
    await s.archive([ref('a2', 't4')])
    expect(s.selectedKey).toBe('a2:t3')
  })

  test('最後の 1 件なら空表示にする', async () => {
    responses.ListInbox = () => page([base[0]].filter((t) => !gone.has(t.threadId)))
    const s = useInboxStore()
    await s.load()
    s.select(ref('a1', 't1'))
    await s.archive([ref('a1', 't1')])
    expect(ids(s)).toEqual([])
    expect(s.selectedKey).toBeUndefined()
    expect(s.cursorKey).toBeUndefined()
  })

  test('ゴミ箱も同じく次の行を開く', async () => {
    const s = useInboxStore()
    await s.load()
    s.select(ref('a1', 't1'))
    await s.trash([ref('a1', 't1')])
    expect(s.selectedKey).toBe('a1:t2')
  })

  test('一括アーカイブは、開いているスレッドを含むときだけ、先頭に除去した行の位置を開く', async () => {
    const s = useInboxStore()
    await s.load()
    s.select(ref('a1', 't2'))
    await s.archive([ref('a1', 't1'), ref('a1', 't2'), ref('a2', 't4')])
    expect(ids(s)).toEqual(['t3'])
    expect(s.selectedKey).toBe('a2:t3')
  })

  test('開いていないスレッドのアーカイブでは開いているスレッドを変えない', async () => {
    const s = useInboxStore()
    await s.load()
    s.select(ref('a1', 't1'))
    await s.archive([ref('a2', 't3')])
    expect(s.selectedKey).toBe('a1:t1')
    await s.archive([ref('a1', 't2'), ref('a2', 't4')])
    expect(s.selectedKey).toBe('a1:t1')
  })

  test('何も開いていないときは何も開かない', async () => {
    const s = useInboxStore()
    await s.load()
    await s.archive([ref('a1', 't2')])
    expect(s.selectedKey).toBeUndefined()
  })

  test('「元に戻す」では開いているスレッドを切り替えない', async () => {
    responses.Unarchive = [base[1]]
    const s = useInboxStore()
    const ui = useUiStore()
    await s.load()
    s.select(ref('a1', 't2'))
    await s.archive([ref('a1', 't2')])
    expect(s.selectedKey).toBe('a2:t3')
    ui.toast?.action?.run()
    await until(() => callsOf('Unarchive').length === 1)
    await until(() => ids(s).includes('t2'))
    expect(s.selectedKey).toBe('a2:t3')
  })

  test('失敗のロールバックでは、まだ次のスレッドを開いているなら元のスレッドを開き直す', async () => {
    failures.Archive = appError('GMAIL_ERROR')
    const s = useInboxStore()
    await s.load()
    s.select(ref('a1', 't2'))
    expect(await s.archive([ref('a1', 't2')])).toBe(false)
    expect(ids(s)).toEqual(['t1', 't2', 't3', 't4'])
    expect(s.selectedKey).toBe('a1:t2')
  })

  test('失敗のロールバックでも、その間にユーザーが開き直したスレッドは変えない', async () => {
    failures.Archive = appError('GMAIL_ERROR')
    const g = deferred()
    gates.Archive = g.p
    const s = useInboxStore()
    await s.load()
    s.select(ref('a1', 't2'))
    const done = s.archive([ref('a1', 't2')])
    s.select(ref('a2', 't4'))
    g.resolve()
    expect(await done).toBe(false)
    expect(s.selectedKey).toBe('a2:t4')
  })

  test('一部だけ失敗したとき、開いていたスレッドが成功していれば次のスレッドのまま', async () => {
    failures.Archive = appError('GMAIL_ERROR', { details: { failedRefs: [ref('a2', 't4')] } })
    const s = useInboxStore()
    await s.load()
    s.select(ref('a1', 't2'))
    gone.add('t2') // t2 はサーバー側で成功済み
    await s.archive([ref('a1', 't2'), ref('a2', 't4')])
    expect(s.selectedKey).toBe('a2:t3')
  })
})
