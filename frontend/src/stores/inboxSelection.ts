// 受信箱リストの選択状態。selectedKey = 開いているスレッド、cursorKey = キーボードカーソル、
// checkedKeys = 複数選択。操作系（e / # など）の対象は targetRefs。
import { computed, type Ref, ref } from 'vue'
import type {
  ThreadRef,
  ThreadSummary,
} from '../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import { cursorAfterRemoval, keyOf, rangeKeys, refFromKey, removeKeys } from '../lib/inboxItems'

export function useInboxSelection(items: Ref<ThreadSummary[]>) {
  const selectedKey = ref<string>()
  const cursorKey = ref<string>()
  const checkedKeys = ref<string[]>([])

  const selectedRef = computed<ThreadRef | undefined>(() =>
    selectedKey.value ? refFromKey(selectedKey.value) : undefined,
  )
  /** 一覧操作の対象: 複数選択があればそれ、なければカーソル（クリック直後は選択行と同じ）の行。 */
  const targetRefs = computed<ThreadRef[]>(() => {
    const keys =
      checkedKeys.value.length > 0 ? checkedKeys.value : cursorKey.value ? [cursorKey.value] : []
    return keys.flatMap((k) => refFromKey(k) ?? [])
  })

  function select(ref?: ThreadRef): void {
    selectedKey.value = ref ? keyOf(ref) : undefined
    cursorKey.value = selectedKey.value ?? cursorKey.value
    checkedKeys.value = []
  }

  function clearSelection(): void {
    selectedKey.value = undefined
    cursorKey.value = undefined
    checkedKeys.value = []
  }

  /** カーソル（キーボード移動の位置）を動かす。extend なら範囲選択を広げる。 */
  function moveCursor(delta: number, extend = false): void {
    const list = items.value
    if (list.length === 0) return
    const cur = list.findIndex((t) => keyOf(t) === cursorKey.value)
    const next = Math.max(
      0,
      Math.min(list.length - 1, cur < 0 ? (delta > 0 ? 0 : list.length - 1) : cur + delta),
    )
    if (extend) {
      const anchor = checkedKeys.value.length > 0 ? checkedKeys.value[0] : cursorKey.value
      checkedKeys.value = rangeKeys(list, anchor, keyOf(list[next]))
    }
    cursorKey.value = keyOf(list[next])
  }

  function openCursor(): void {
    if (cursorKey.value) select(refFromKey(cursorKey.value))
  }

  /** Ctrl+クリック: 複数選択に加える／外す。 */
  function toggleChecked(ref: ThreadRef): void {
    const k = keyOf(ref)
    const base =
      checkedKeys.value.length > 0 ? checkedKeys.value : cursorKey.value ? [cursorKey.value] : []
    checkedKeys.value = base.includes(k) ? base.filter((x) => x !== k) : [...base, k]
    cursorKey.value = k
  }

  /** Shift+クリック: カーソルから対象までの範囲を選択する。 */
  function extendChecked(ref: ThreadRef): void {
    const to = keyOf(ref)
    checkedKeys.value = rangeKeys(items.value, cursorKey.value, to)
    cursorKey.value = to
  }

  /** 一覧から消えた行を選択から外す（開いているスレッドは一覧に無くても維持する）。 */
  function pruneSelection(): void {
    const keys = new Set(items.value.map(keyOf))
    if (checkedKeys.value.some((k) => !keys.has(k)))
      checkedKeys.value = checkedKeys.value.filter((k) => keys.has(k))
    if (cursorKey.value && !keys.has(cursorKey.value) && cursorKey.value !== selectedKey.value)
      cursorKey.value = undefined
  }

  /**
   * 行の除去に伴う選択の更新。開いているスレッドが除去対象のときの扱いは `opened` で決める:
   * 'next' = 除去した最初の位置の次の行（末尾なら一つ前。無ければ空表示）を開く、'close' = 閉じる、
   * 'keep' = 開いたまま。開いていないスレッドの除去では開いているスレッドを変えない。
   * 戻り値は失敗時のロールバック用。'next' で開いているスレッドを切り替えた（次の行を開いた・次が無くて空表示にした）後、
   * まだそのままなら（`failed` があればその中に元のスレッドがあるときだけ）元のスレッドを開き直す。
   */
  function afterRemoval(
    keys: ReadonlySet<string>,
    kept: ThreadSummary[],
    firstIndex: number,
    opened: 'next' | 'close' | 'keep',
  ): (failed?: Set<string>) => void {
    checkedKeys.value = checkedKeys.value.filter((k) => !keys.has(k))
    const prev = selectedKey.value
    let switched = false
    let autoOpened: string | undefined
    if (opened !== 'keep' && prev && keys.has(prev)) {
      switched = true
      autoOpened = opened === 'next' ? cursorAfterRemoval(kept, firstIndex) : undefined
      selectedKey.value = autoOpened
    }
    if (cursorKey.value && keys.has(cursorKey.value) && cursorKey.value !== selectedKey.value) {
      cursorKey.value = cursorAfterRemoval(kept, firstIndex)
    }
    return (failed) => {
      if (!switched || !prev || selectedKey.value !== autoOpened) return
      if (failed && !failed.has(prev)) return
      selectedKey.value = prev
      cursorKey.value = prev
    }
  }

  /** サーバー呼び出しなしでリストから除去する（スレッドが NOT_FOUND だった場合など）。 */
  function removeThread(ref: ThreadRef): void {
    const keys = new Set([keyOf(ref)])
    const r = removeKeys(items.value, keys)
    items.value = r.items
    afterRemoval(keys, r.items, r.removed[0]?.index ?? 0, 'close')
  }

  return {
    selectedKey,
    cursorKey,
    checkedKeys,
    selectedRef,
    targetRefs,
    select,
    clearSelection,
    moveCursor,
    openCursor,
    toggleChecked,
    extendChecked,
    pruneSelection,
    afterRemoval,
    removeThread,
  }
}
