import { defineStore } from 'pinia'
import { computed, ref, watch } from 'vue'
import { layoutModeFor, MEDIUM_QUERY, WIDE_QUERY } from '../lib/layout'
import { usePlatformStore } from './platform'
import { useUiStore } from './ui'

/**
 * 画面幅によるレイアウト（3 ペイン／2 ペイン／1 画面ずつ）の唯一の判定元。幅の監視は matchMedia のここだけで、
 * ブレイクポイントは lib/layout.ts の定数。タッチ UI でない OS（Windows）では常に wide。
 */
export const useLayoutStore = defineStore('layout', () => {
  const platform = usePlatformStore()
  const ui = useUiStore()

  // 監視の開始前は wide（matchMedia が無い環境も同じ）。
  const wideMatches = ref(true)
  const mediumMatches = ref(true)
  let stop: (() => void) | undefined

  const mode = computed(() =>
    layoutModeFor(platform.touchUi, wideMatches.value, mediumMatches.value),
  )
  const isWide = computed(() => mode.value === 'wide')
  const isStack = computed(() => mode.value === 'stack')
  /** サイドバーを常時表示せず、ドロワーで開くレイアウトか。 */
  const hasDrawer = computed(() => mode.value !== 'wide')

  /** 幅の監視と、CSS 用の属性（html の data-touch-ui・data-layout）の反映を始める（冪等）。 */
  function start(): void {
    if (stop) return
    const wide = window.matchMedia?.(WIDE_QUERY)
    const medium = window.matchMedia?.(MEDIUM_QUERY)
    const sync = (): void => {
      wideMatches.value = wide?.matches ?? true
      mediumMatches.value = medium?.matches ?? true
    }
    sync()
    wide?.addEventListener('change', sync)
    medium?.addEventListener('change', sync)
    const stopAttrs = watch(
      [mode, () => platform.touchUi],
      ([m, touch]) => {
        const root = document.documentElement
        root.toggleAttribute('data-touch-ui', touch)
        root.dataset.layout = m
        // ドロワーを使わない幅になったら閉じておく（再び狭くしたとき開いたままにならない）
        if (m === 'wide') ui.closeMenu()
      },
      { immediate: true, flush: 'sync' },
    )
    stop = () => {
      wide?.removeEventListener('change', sync)
      medium?.removeEventListener('change', sync)
      stopAttrs()
      stop = undefined
    }
  }

  function dispose(): void {
    stop?.()
  }

  return { mode, isWide, isStack, hasDrawer, start, dispose }
})
