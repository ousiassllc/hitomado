// 重ねて出すスクロールバーが「いま見えているか」の状態機械（#378）。DOM を知らない。
// 出す: エリアにマウスが乗った／つまみをドラッグしている／フォーカスがある（hold）、スクロールした（activity）、開いた（flash）。
// 消す: hold が全部外れ、最後の活動から待ち時間（800ms。flash は 1 秒）が過ぎたとき。見えている間にタイマーが切れても、hold が
// あれば消さない。タイマーはここだけが持ち、destroy で必ず解放する。

/** 消えるまでの待ち（スクロール停止・マウスが外へ出てから）。reduced-motion でも同じ。 */
export const HIDE_DELAY_MS = 800
/** エリアが開いたとき、はみ出していることを知らせるために出しておく時間。 */
export const FLASH_MS = 1000

export interface Clock {
  set(fn: () => void, ms: number): unknown
  clear(handle: unknown): void
}

export const browserClock: Clock = {
  set: (fn, ms) => setTimeout(fn, ms),
  clear: (handle) => clearTimeout(handle as ReturnType<typeof setTimeout>),
}

export type Hold = 'hover' | 'focus' | 'drag'

export class Presence {
  private shown = false
  private readonly holds = new Set<Hold>()
  private timer: unknown

  constructor(
    private readonly clock: Clock,
    private readonly onChange: (shown: boolean) => void,
  ) {}

  get visible(): boolean {
    return this.shown
  }

  /** マウスが乗った・ドラッグ中・フォーカス中。外れるまで消さない。 */
  hold(kind: Hold): void {
    this.holds.add(kind)
    this.cancel()
    this.setShown(true)
  }

  release(kind: Hold): void {
    if (!this.holds.delete(kind) || this.holds.size > 0) return
    if (this.shown) this.arm(HIDE_DELAY_MS)
  }

  /** スクロールした。出して、止まってから待ち時間のあとに消す。 */
  activity(): void {
    this.setShown(true)
    this.arm(HIDE_DELAY_MS)
  }

  /** エリアが開いた。1 秒だけ出してから消す。 */
  flash(): void {
    this.setShown(true)
    this.arm(FLASH_MS)
  }

  destroy(): void {
    this.cancel()
    this.holds.clear()
  }

  private arm(ms: number): void {
    this.cancel()
    if (this.holds.size > 0) return
    this.timer = this.clock.set(() => {
      this.timer = undefined
      if (this.holds.size === 0) this.setShown(false)
    }, ms)
  }

  private cancel(): void {
    if (this.timer === undefined) return
    this.clock.clear(this.timer)
    this.timer = undefined
  }

  private setShown(next: boolean): void {
    if (this.shown === next) return
    this.shown = next
    this.onChange(next)
  }
}
