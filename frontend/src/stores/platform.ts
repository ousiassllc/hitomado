import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import {
  type Capabilities,
  PlatformService,
} from '../../bindings/github.com/ousiassllc/hitomado/internal/capability'
import { call, parseBindingError } from '../lib/api'
import { settings as msg } from '../lib/messages'
import { useUiStore } from './ui'

/**
 * 読み込み前・読み込みに失敗したときの値（デスクトップ＝Windows 版の現在の動作）。
 * Android（MCP・トレイ・自動更新などが無い OS）では、起動時の読み込みで上書きされる。
 */
const DEFAULT_CAPABILITIES: Capabilities = {
  os: '',
  supportsMcp: true,
  supportsTray: true,
  supportsAutoUpdate: true,
  supportsWindowState: true,
  singleInstance: true,
  touchUi: false,
}

/**
 * この OS でアプリが提供する機能（Go の PlatformService.GetCapabilities）。
 * 画面の出し分けは OS 名ではなくここの機能の有無で決める（モバイル UI も同じ値を使う）。
 */
export const usePlatformStore = defineStore('platform', () => {
  const ui = useUiStore()
  const capabilities = ref<Capabilities>({ ...DEFAULT_CAPABILITIES })
  const loaded = ref(false)
  /** 読み込みが終わった（成功・失敗とも）。画面の構成はこれ以降に決める（Android で一瞬 3 ペインを出さない）。 */
  const settled = ref(false)
  let started = false

  const supportsMcp = computed(() => capabilities.value.supportsMcp)
  const supportsTray = computed(() => capabilities.value.supportsTray)
  const supportsAutoUpdate = computed(() => capabilities.value.supportsAutoUpdate)
  const supportsWindowState = computed(() => capabilities.value.supportsWindowState)
  const singleInstance = computed(() => capabilities.value.singleInstance)
  /** タッチ操作中心の画面（幅に応じて 1 画面ずつ進む表示・44px のタップ領域）。Android だけ true。 */
  const touchUi = computed(() => capabilities.value.touchUi)

  /** 冪等。起動時に 1 回だけ読む。失敗時はエラートーストを出して既定値（デスクトップ）のままにする。 */
  async function start(): Promise<void> {
    if (started) return
    started = true
    try {
      capabilities.value = await call(PlatformService.GetCapabilities())
      loaded.value = true
    } catch (e) {
      ui.pushToast('error', parseBindingError(e).message || msg.loadFailed)
    } finally {
      settled.value = true
    }
  }

  return {
    capabilities,
    loaded,
    settled,
    supportsMcp,
    supportsTray,
    supportsAutoUpdate,
    supportsWindowState,
    singleInstance,
    touchUi,
    start,
  }
})
