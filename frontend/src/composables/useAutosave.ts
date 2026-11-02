// 下書きの自動保存（components/overview.md §3.7）。編集停止後 3 秒でデバウンスして保存する。
// - 送信元未選択（canSave=false）・保存中は呼ばない。保存中の編集は完了後にもう一度保存する。
// - 世代番号で最新のみ採用: 連続編集中に完了した古い保存結果では「保存済み」にしない。
// - タイマーは注入可能（テストで固定時間待ちをしない）。
import { getCurrentScope, onScopeDispose } from 'vue'

export const AUTOSAVE_DELAY_MS = 3000

export interface AutosaveTimer {
  set(fn: () => void, ms: number): unknown
  clear(handle: unknown): void
}

const defaultTimer: AutosaveTimer = {
  set: (fn, ms) => setTimeout(fn, ms),
  clear: (handle) => clearTimeout(handle as ReturnType<typeof setTimeout>),
}

export interface AutosaveOptions<S, R> {
  /** 保存を呼べる状態か（送信元選択済み・送信中でない 等）。 */
  canSave: () => boolean
  /** 実際の保存。snapshot は notify 時点の内容。 */
  save: (snapshot: S) => Promise<R>
  /** 最新世代の保存が成功したときだけ呼ばれる（古い結果では呼ばれない）。 */
  onSaved?: (result: R) => void
  /** 保存の失敗。 */
  onError?: (error: unknown) => void
  delayMs?: number
  timer?: AutosaveTimer
}

export interface Autosave<S> {
  /** 内容が変わったことを知らせる。デバウンスを張り直す。 */
  notify(snapshot: S): void
  /** タイマーを待たず、未保存があれば保存して完了を待つ。全て保存済みなら true。 */
  flush(): Promise<boolean>
  /** 未保存の予約を捨て、実行中の保存の完了を待つ（破棄・送信の前に使う）。 */
  drain(): Promise<void>
  /** 未保存の内容があるか。 */
  readonly pending: boolean
}

export function useAutosave<S, R>(opts: AutosaveOptions<S, R>): Autosave<S> {
  const delay = opts.delayMs ?? AUTOSAVE_DELAY_MS
  const timer = opts.timer ?? defaultTimer
  let gen = 0
  let savedGen = 0
  let snapshot: S | undefined
  let handle: unknown
  let inflight: Promise<boolean> | undefined

  function clearTimer(): void {
    if (handle === undefined) return
    timer.clear(handle)
    handle = undefined
  }

  function schedule(): void {
    clearTimer()
    handle = timer.set(() => {
      handle = undefined
      void run()
    }, delay)
  }

  /** 保存を 1 回実行する。実行中ならその完了を返す。保存した/不要なら true、失敗なら false。 */
  function run(): Promise<boolean> {
    if (inflight) return inflight
    if (gen === savedGen || snapshot === undefined || !opts.canSave()) return Promise.resolve(true)
    const g = gen
    const s = snapshot
    inflight = (async () => {
      let ok = true
      try {
        const result = await opts.save(s)
        if (g === gen) {
          savedGen = g
          opts.onSaved?.(result)
        }
      } catch (e) {
        ok = false
        opts.onError?.(e)
      } finally {
        inflight = undefined
      }
      // 保存中に来た編集（タイマーが保存中に切れた分を含む）は、最後の編集から改めてデバウンスする。
      if (ok && gen !== savedGen && handle === undefined) schedule()
      return ok
    })()
    return inflight
  }

  async function flush(): Promise<boolean> {
    clearTimer()
    for (;;) {
      if (inflight && !(await inflight)) return false
      if (gen === savedGen || snapshot === undefined) return true
      if (!opts.canSave()) return false
      if (!(await run())) return false
      clearTimer()
    }
  }

  async function drain(): Promise<void> {
    clearTimer()
    savedGen = gen
    if (inflight) await inflight
    clearTimer()
  }

  const api: Autosave<S> = {
    notify(s) {
      snapshot = s
      gen++
      schedule()
    },
    flush,
    drain,
    get pending() {
      return gen !== savedGen
    },
  }

  if (getCurrentScope()) onScopeDispose(clearTimer)
  return api
}
