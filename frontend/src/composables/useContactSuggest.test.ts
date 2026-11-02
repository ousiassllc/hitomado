import { describe, expect, test } from 'bun:test'
import type { Address } from '../../bindings/github.com/ousiassllc/hitomado/internal/domain/model'
import { type SuggestTimer, useContactSuggest } from './useContactSuggest'

class FakeTimer implements SuggestTimer {
  private next = 1
  tasks = new Map<number, { fn: () => void; ms: number }>()
  set(fn: () => void, ms: number): unknown {
    const id = this.next++
    this.tasks.set(id, { fn, ms })
    return id
  }
  clear(handle: unknown): void {
    this.tasks.delete(handle as number)
  }
  fire(): void {
    const due = [...this.tasks.values()]
    this.tasks.clear()
    for (const t of due) t.fn()
  }
}

function setup(opts: { accountId?: string } = { accountId: 'a1' }) {
  const { accountId } = opts
  const timer = new FakeTimer()
  const fetches: {
    accountId: string
    prefix: string
    limit: number
    d: PromiseWithResolvers<Address[] | null>
  }[] = []
  const s = useContactSuggest({
    accountId: () => accountId,
    timer,
    fetch: (a, prefix, limit) => {
      const d = Promise.withResolvers<Address[] | null>()
      fetches.push({ accountId: a, prefix, limit, d })
      return d.promise
    },
  })
  return { timer, fetches, ...s }
}

const flush = () => new Promise<void>((r) => queueMicrotask(r))

describe('useContactSuggest', () => {
  test('150ms のデバウンスで最後の入力だけ問い合わせる', async () => {
    const { timer, fetches, query, suggestions } = setup()
    query('a')
    query('ab')
    expect([...timer.tasks.values()].map((t) => t.ms)).toEqual([150])
    timer.fire()
    expect(fetches.map((f) => [f.accountId, f.prefix])).toEqual([['a1', 'ab']])
    fetches[0].d.resolve([{ email: 'abc@x.example' }])
    await flush()
    expect(suggestions.value).toEqual([{ email: 'abc@x.example' }])
  })

  test('送信元未選択・空入力では問い合わせず、候補を消す', async () => {
    const none = setup({})
    none.query('a')
    none.timer.fire()
    expect(none.fetches).toHaveLength(0)

    const { timer, fetches, query, suggestions } = setup()
    query('a')
    timer.fire()
    fetches[0].d.resolve([{ email: 'a@x.example' }])
    await flush()
    query('  ')
    expect(suggestions.value).toEqual([])
    timer.fire()
    expect(fetches).toHaveLength(1)
  })

  test('古い応答は新しい入力の候補を上書きしない', async () => {
    const { timer, fetches, query, suggestions } = setup()
    query('a')
    timer.fire()
    query('ab')
    timer.fire()
    fetches[1].d.resolve([{ email: 'new@x.example' }])
    await flush()
    fetches[0].d.resolve([{ email: 'old@x.example' }])
    await flush()
    expect(suggestions.value).toEqual([{ email: 'new@x.example' }])
  })

  test('clear は進行中の応答も無効にする。失敗時は候補なし', async () => {
    const { timer, fetches, query, clear, suggestions } = setup()
    query('a')
    timer.fire()
    clear()
    fetches[0].d.resolve([{ email: 'late@x.example' }])
    await flush()
    expect(suggestions.value).toEqual([])
    query('b')
    timer.fire()
    fetches[1].d.reject(new Error('x'))
    await flush()
    expect(suggestions.value).toEqual([])
  })
})
