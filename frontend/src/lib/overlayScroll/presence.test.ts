import { describe, expect, test } from 'bun:test'
import { type Clock, FLASH_MS, HIDE_DELAY_MS, Presence } from './presence'

/** 手で進める時計。タイマーの残りを数えられる（解放漏れの検証用）。 */
class FakeClock implements Clock {
  now = 0
  private next = 1
  private readonly timers = new Map<number, { at: number; fn: () => void }>()
  set(fn: () => void, ms: number): unknown {
    const id = this.next++
    this.timers.set(id, { at: this.now + ms, fn })
    return id
  }
  clear(handle: unknown): void {
    this.timers.delete(handle as number)
  }
  get pending(): number {
    return this.timers.size
  }
  advance(ms: number): void {
    const end = this.now + ms
    for (;;) {
      const due = [...this.timers.entries()]
        .filter(([, t]) => t.at <= end)
        .sort((a, b) => a[1].at - b[1].at)[0]
      if (!due) break
      this.timers.delete(due[0])
      this.now = due[1].at
      due[1].fn()
    }
    this.now = end
  }
}

function setup() {
  const clock = new FakeClock()
  const changes: boolean[] = []
  const presence = new Presence(clock, (shown) => changes.push(shown))
  return { clock, changes, presence }
}

describe('Presence', () => {
  test('スクロールすると出て、止まって 800ms 後に消える', () => {
    const { clock, presence, changes } = setup()
    presence.activity()
    expect(presence.visible).toBe(true)
    clock.advance(HIDE_DELAY_MS - 1)
    expect(presence.visible).toBe(true)
    clock.advance(1)
    expect(presence.visible).toBe(false)
    expect(changes).toEqual([true, false])
  })

  test('スクロールが続く間は待ち時間が延びる（最後のスクロールから 800ms）', () => {
    const { clock, presence } = setup()
    presence.activity()
    clock.advance(500)
    presence.activity()
    clock.advance(HIDE_DELAY_MS - 1)
    expect(presence.visible).toBe(true)
    clock.advance(1)
    expect(presence.visible).toBe(false)
  })

  test('マウスが中にいる間は消さない。外に出てから 800ms 待って消す', () => {
    const { clock, presence } = setup()
    presence.hold('hover')
    clock.advance(10_000)
    expect(presence.visible).toBe(true)
    presence.release('hover')
    clock.advance(HIDE_DELAY_MS - 1)
    expect(presence.visible).toBe(true)
    clock.advance(1)
    expect(presence.visible).toBe(false)
  })

  test('スクロール中にマウスが中にいるなら、スクロールが止まっても消さない', () => {
    const { clock, presence } = setup()
    presence.hold('hover')
    presence.activity()
    clock.advance(HIDE_DELAY_MS * 3)
    expect(presence.visible).toBe(true)
  })

  test('つまみのドラッグ中・フォーカス中も消さない。全部外れてから 800ms', () => {
    const { clock, presence } = setup()
    presence.hold('hover')
    presence.hold('drag')
    presence.hold('focus')
    presence.release('hover')
    presence.release('drag')
    clock.advance(10_000)
    expect(presence.visible).toBe(true)
    presence.release('focus')
    clock.advance(HIDE_DELAY_MS)
    expect(presence.visible).toBe(false)
  })

  test('出てから消すまでの間にマウスが戻れば消えない（タイマーを取り消す）', () => {
    const { clock, presence } = setup()
    presence.hold('hover')
    presence.release('hover')
    clock.advance(HIDE_DELAY_MS - 100)
    presence.hold('hover')
    expect(clock.pending).toBe(0)
    clock.advance(10_000)
    expect(presence.visible).toBe(true)
  })

  test('開いたときの予告は 1 秒だけ出して消す', () => {
    const { clock, presence } = setup()
    presence.flash()
    expect(presence.visible).toBe(true)
    clock.advance(FLASH_MS - 1)
    expect(presence.visible).toBe(true)
    clock.advance(1)
    expect(presence.visible).toBe(false)
  })

  test('予告の間にマウスが乗れば、外れるまで消さない', () => {
    const { clock, presence } = setup()
    presence.flash()
    clock.advance(300)
    presence.hold('hover')
    clock.advance(FLASH_MS)
    expect(presence.visible).toBe(true)
  })

  test('hold していないものの release は何もしない', () => {
    const { clock, presence, changes } = setup()
    presence.release('hover')
    expect(clock.pending).toBe(0)
    expect(changes).toEqual([])
  })

  test('destroy でタイマーを残さない（リーク禁止）', () => {
    const { clock, presence, changes } = setup()
    presence.activity()
    presence.destroy()
    expect(clock.pending).toBe(0)
    clock.advance(10_000)
    expect(changes).toEqual([true])
  })
})
