import { describe, expect, test } from 'bun:test'
import { type AutosaveTimer, useAutosave } from './useAutosave'

/** 手動で進める偽タイマー。実時間を待たない。 */
class FakeTimer implements AutosaveTimer {
  private next = 1
  private tasks = new Map<number, { fn: () => void; ms: number }>()
  set(fn: () => void, ms: number): unknown {
    const id = this.next++
    this.tasks.set(id, { fn, ms })
    return id
  }
  clear(handle: unknown): void {
    this.tasks.delete(handle as number)
  }
  get count(): number {
    return this.tasks.size
  }
  get delays(): number[] {
    return [...this.tasks.values()].map((t) => t.ms)
  }
  fireAll(): void {
    const due = [...this.tasks.entries()]
    this.tasks.clear()
    for (const [, t] of due) t.fn()
  }
}

function deferred<T>(): PromiseWithResolvers<T> {
  return Promise.withResolvers<T>()
}

function setup(canSave: () => boolean = () => true) {
  const timer = new FakeTimer()
  const saves: { snapshot: string; d: PromiseWithResolvers<number> }[] = []
  const saved: number[] = []
  const errors: unknown[] = []
  const autosave = useAutosave<string, number>({
    canSave,
    timer,
    save: (snapshot) => {
      const d = deferred<number>()
      saves.push({ snapshot, d })
      return d.promise
    },
    onSaved: (r) => saved.push(r),
    onError: (e) => errors.push(e),
  })
  return { timer, saves, saved, errors, autosave }
}

const tick = () => new Promise<void>((r) => queueMicrotask(r))

describe('useAutosave', () => {
  test('編集停止後 3 秒（既定）のデバウンスで保存し、連続編集では張り直す', async () => {
    const { timer, saves, autosave } = setup()
    autosave.notify('a')
    autosave.notify('ab')
    expect(timer.count).toBe(1)
    expect(timer.delays).toEqual([3000])
    expect(saves).toHaveLength(0)
    timer.fireAll()
    await tick()
    expect(saves.map((s) => s.snapshot)).toEqual(['ab'])
  })

  test('canSave が false（送信元未選択）なら保存を呼ばない', async () => {
    let ok = false
    const { timer, saves, autosave } = setup(() => ok)
    autosave.notify('a')
    timer.fireAll()
    await tick()
    expect(saves).toHaveLength(0)
    expect(autosave.pending).toBe(true)
    ok = true
    const p = autosave.flush()
    await tick()
    expect(saves).toHaveLength(1)
    saves[0].d.resolve(1)
    expect(await p).toBe(true)
  })

  test('保存中は新たに呼ばず、完了後に最新内容をもう一度保存する', async () => {
    const { timer, saves, saved, autosave } = setup()
    autosave.notify('a')
    timer.fireAll()
    await tick()
    expect(saves).toHaveLength(1)

    autosave.notify('ab')
    timer.fireAll() // 保存中にタイマーが切れても二重に呼ばない
    await tick()
    expect(saves).toHaveLength(1)

    saves[0].d.resolve(1)
    await tick()
    await tick()
    // 古い結果（世代 1）は採用されず、改めてデバウンスされる
    expect(saved).toEqual([])
    expect(timer.count).toBe(1)
    timer.fireAll()
    await tick()
    expect(saves.map((s) => s.snapshot)).toEqual(['a', 'ab'])
    saves[1].d.resolve(2)
    await tick()
    await tick()
    expect(saved).toEqual([2])
    expect(autosave.pending).toBe(false)
  })

  test('古い保存結果は新しい内容を「保存済み」にしない（世代番号）', async () => {
    const { timer, saves, saved, autosave } = setup()
    autosave.notify('a')
    timer.fireAll()
    await tick()
    autosave.notify('ab')
    saves[0].d.resolve(1)
    await tick()
    await tick()
    expect(saved).toEqual([])
    expect(autosave.pending).toBe(true)
  })

  test('flush は未保存があれば直ちに保存して完了を待つ', async () => {
    const { timer, saves, saved, autosave } = setup()
    autosave.notify('a')
    const p = autosave.flush()
    await tick()
    expect(saves).toHaveLength(1)
    expect(timer.count).toBe(0)
    saves[0].d.resolve(7)
    expect(await p).toBe(true)
    expect(saved).toEqual([7])
    expect(autosave.pending).toBe(false)
  })

  test('flush は保存中に来た編集も含めて最後まで保存する', async () => {
    const { saves, saved, autosave } = setup()
    autosave.notify('a')
    const p = autosave.flush()
    await tick()
    autosave.notify('ab')
    saves[0].d.resolve(1)
    await tick()
    await tick()
    expect(saves.map((s) => s.snapshot)).toEqual(['a', 'ab'])
    saves[1].d.resolve(2)
    expect(await p).toBe(true)
    expect(saved).toEqual([2])
  })

  test('未保存が無ければ flush は何も呼ばない', async () => {
    const { saves, autosave } = setup()
    expect(await autosave.flush()).toBe(true)
    expect(saves).toHaveLength(0)
  })

  test('保存失敗は onError に渡し、flush は false。再度の flush で再試行できる', async () => {
    const { saves, saved, errors, autosave } = setup()
    autosave.notify('a')
    const p = autosave.flush()
    await tick()
    saves[0].d.reject(new Error('boom'))
    expect(await p).toBe(false)
    expect(errors).toHaveLength(1)
    expect(autosave.pending).toBe(true)

    const p2 = autosave.flush()
    await tick()
    expect(saves).toHaveLength(2)
    saves[1].d.resolve(3)
    expect(await p2).toBe(true)
    expect(saved).toEqual([3])
  })

  test('失敗後は新しい編集があるまで自動では再試行しない', async () => {
    const { timer, saves, autosave } = setup()
    autosave.notify('a')
    timer.fireAll()
    await tick()
    saves[0].d.reject(new Error('boom'))
    await tick()
    await tick()
    expect(timer.count).toBe(0)
    autosave.notify('ab')
    expect(timer.count).toBe(1)
  })

  test('drain は予約を捨て、実行中の保存の完了を待つ', async () => {
    const { timer, saves, autosave } = setup()
    autosave.notify('a')
    timer.fireAll()
    await tick()
    autosave.notify('ab')
    let done = false
    const p = autosave.drain().then(() => {
      done = true
    })
    await tick()
    expect(done).toBe(false)
    saves[0].d.resolve(1)
    await p
    expect(timer.count).toBe(0)
    expect(saves).toHaveLength(1)
    expect(autosave.pending).toBe(false)
  })
})
