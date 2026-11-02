// 宛先オートコンプリート（F-13）。入力が止まって 150ms で SuggestContacts を呼ぶ。
// 候補は送信元アカウントごと。古い応答は捨てる。タイマーは注入可能（テストで固定時間待ちをしない）。
import { getCurrentScope, onScopeDispose, ref } from 'vue'
import type { Address } from '../../bindings/github.com/ousiassllc/hitomado/internal/domain/model'

export const SUGGEST_DELAY_MS = 150
export const SUGGEST_LIMIT = 8

export interface SuggestTimer {
  set(fn: () => void, ms: number): unknown
  clear(handle: unknown): void
}

const defaultTimer: SuggestTimer = {
  set: (fn, ms) => setTimeout(fn, ms),
  clear: (handle) => clearTimeout(handle as ReturnType<typeof setTimeout>),
}

export interface ContactSuggestOptions {
  /** 現在の送信元アカウント ID。未選択なら候補は出さない。 */
  accountId: () => string | undefined
  fetch: (accountId: string, prefix: string, limit: number) => Promise<Address[] | null>
  timer?: SuggestTimer
}

export function useContactSuggest(opts: ContactSuggestOptions) {
  const timer = opts.timer ?? defaultTimer
  const suggestions = ref<Address[]>([])
  let handle: unknown
  let seq = 0

  function clearTimer(): void {
    if (handle === undefined) return
    timer.clear(handle)
    handle = undefined
  }

  /** 候補を消す。進行中の応答も無効にする。 */
  function clear(): void {
    clearTimer()
    seq++
    suggestions.value = []
  }

  /** 入力が変わった。空・送信元未選択なら候補を消す。 */
  function query(raw: string): void {
    const prefix = raw.trim()
    const account = opts.accountId()
    clearTimer()
    if (prefix === '' || account === undefined) {
      clear()
      return
    }
    const mine = ++seq
    handle = timer.set(() => {
      handle = undefined
      opts
        .fetch(account, prefix, SUGGEST_LIMIT)
        .then((list) => {
          if (mine === seq) suggestions.value = list ?? []
        })
        .catch(() => {
          // 候補は補助機能。取得に失敗しても入力は妨げない。
          if (mine === seq) suggestions.value = []
        })
    }, SUGGEST_DELAY_MS)
  }

  if (getCurrentScope()) onScopeDispose(clear)
  return { suggestions, query, clear }
}
