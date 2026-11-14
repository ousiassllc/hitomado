import { defineStore } from 'pinia'
import { computed, ref, watch } from 'vue'
import { layoutModeFor, MEDIUM_QUERY, WIDE_QUERY } from '../lib/layout'
import { usePlatformStore } from './platform'

/**
 * 画面幅によるレイアウト（3 ペイン／2 ペイン／1 画面ずつ）の唯一の判定元。幅の監視は matchMedia のここだけで、
 * ブレイクポイントは lib/layout.ts の定数。タッチ UI でない OS（Windows）では常に wide。
 */
export const useLayoutStore = defineStore('layout', () => {
  const platform = usePlatformStore()

  // 監視の開始前は wide（matchMedia が無い環境も同じ）。
  const wideMatches = ref(true)
  const mediumMatches = ref(true)
  let stop: (() => void) | undefined

  const mode = computed(() =>
    layoutModeFor(platform.touchUi, wideMatches.value, mediumMatches.value),
  )
  const isWide = computed(() => mode.value === 'wide')
  const isStack = computed(() => mode.value === 'stack')
  const hasTabBar = computed(() => mode.value !== 'wide') // サイドバーの代わりに、下 4 タブとアカウントのチップを使うレイアウト

  /** 幅の監視と、CSS 用の属性（html の data-touch-ui・data-layout）の反映を始める（冪等）。 */
  function start(): void {
    if (stop) return
    const queries = [WIDE_QUERY, MEDIUM_QUERY].map((q) => window.matchMedia?.(q))
    const sync = (): void => {
      wideMatches.value = queries[0]?.matches ?? true
      mediumMatches.value = queries[1]?.matches ?? true
    }
    sync()
    for (const q of queries) q?.addEventListener('change', sync)
    const stopAttrs = watch(
      [mode, () => platform.touchUi],
      ([m, touch]) => {
        const root = document.documentElement
        root.toggleAttribute('data-touch-ui', touch)
        root.dataset.layout = m
      },
      { immediate: true, flush: 'sync' },
    )
    stop = () => {
      for (const q of queries) q?.removeEventListener('change', sync)
      stopAttrs()
      stop = undefined
    }
  }

  function dispose(): void {
    stop?.()
  }

  return { mode, isWide, isStack, hasTabBar, start, dispose }
})
