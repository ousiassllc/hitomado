<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useNavigation } from '../../composables/useNavigation'
import { useShortcuts } from '../../composables/useShortcuts'
import { installBackHandler } from '../../lib/androidBack'
import { common } from '../../lib/messages'
import { clampList, clampSidebar, LIST, SIDEBAR } from '../../lib/paneWidth'
import { useComposeStore } from '../../stores/compose'
import { useLayoutStore } from '../../stores/layout'
import { usePlatformStore } from '../../stores/platform'
import { useUiStore } from '../../stores/ui'
import AddAccountDialog from '../account/AddAccountDialog.vue'
import ComposePane from '../compose/ComposePane.vue'
import SendConfirmDialog from '../compose/SendConfirmDialog.vue'
import InboxZeroDialog from '../inbox/InboxZeroDialog.vue'
import ThreadListPane from '../inbox/ThreadListPane.vue'
import SettingsDialog from '../settings/SettingsDialog.vue'
import ThreadPane from '../thread/ThreadPane.vue'
import AlertStack from './AlertStack.vue'
import BannerStack from './BannerStack.vue'
import PaneSplitter from './PaneSplitter.vue'
import Sidebar from './Sidebar.vue'
import ToastHost from './ToastHost.vue'

const compose = useComposeStore()
const layout = useLayoutStore()
const platform = usePlatformStore()
const ui = useUiStore()
const { screen, goBack } = useNavigation()
useShortcuts()

const total = ref(window.innerWidth)
const sidebarWidth = ref<number>(SIDEBAR.initial)
const listWidth = ref<number>(LIST.initial)

/** wide は 3 ペイン、medium はサイドバーをドロワーにした 2 ペイン、stack は 1 画面ずつ（一覧・スレッド・作成が同じ場所に重なる）。 */
const columns = computed(() => {
  switch (layout.mode) {
    case 'wide':
      return `${sidebarWidth.value}px ${listWidth.value}px minmax(0, 1fr)`
    case 'medium':
      return `${listWidth.value}px minmax(0, 1fr)`
    default:
      return 'minmax(0, 1fr)'
  }
})
/** 新規作成は右ペイン全面に ComposePane（ThreadPane は隠す）。返信・転送は ThreadPane 内に埋め込まれる。 */
const composingNew = computed(() => compose.session?.mode === 'new')
/** 1 画面ずつのとき、作成・返信は画面全体を覆う ComposePane（下の ThreadPane は残して、戻ったときに読んでいた位置を保つ）。 */
const covering = computed(() => screen.value === 'compose' || screen.value === 'reply')

function onWindowResize(): void {
  total.value = window.innerWidth
  sidebarWidth.value = clampSidebar(sidebarWidth.value, listWidth.value, total.value)
  listWidth.value = clampList(listWidth.value, inlineSidebarWidth(), total.value)
}

/** 一覧の幅の計算に使うサイドバーの幅（ドロワーのときは 0）。 */
function inlineSidebarWidth(): number {
  return layout.isWide ? sidebarWidth.value : 0
}

function resizeSidebar(delta: number): void {
  sidebarWidth.value = clampSidebar(sidebarWidth.value + delta, listWidth.value, total.value)
}

function resizeList(delta: number): void {
  listWidth.value = clampList(listWidth.value + delta, inlineSidebarWidth(), total.value)
}

let removeBackHandler: (() => void) | undefined

onMounted(() => {
  window.addEventListener('resize', onWindowResize)
  layout.start()
  // Android の戻るボタン（MainActivity.java が呼ぶ）。タッチ UI の OS だけ受ける。
  if (platform.touchUi) removeBackHandler = installBackHandler(goBack)
})
onBeforeUnmount(() => {
  window.removeEventListener('resize', onWindowResize)
  layout.dispose()
  removeBackHandler?.()
})
</script>

<template>
  <div class="shell" :class="`layout-${layout.mode}`">
    <BannerStack />
    <!-- ダイアログが開いている間は、アラートはダイアログの中に出る（BaseDialog） -->
    <AlertStack v-if="ui.overlays.length === 0" class="shell-alerts" />
    <div class="panes" :style="{ gridTemplateColumns: columns }">
      <div v-if="layout.isWide" class="col">
        <Sidebar />
        <PaneSplitter
          class="split split-1"
          :label="common.shell.sidebarResize"
          :value="sidebarWidth"
          :min="SIDEBAR.min"
          :max="SIDEBAR.max"
          @resize="resizeSidebar"
        />
      </div>
      <div class="col" :class="{ off: layout.isStack && screen !== 'list' }">
        <ThreadListPane />
        <PaneSplitter
          v-if="!layout.isStack"
          class="split split-2"
          :label="common.shell.listResize"
          :value="listWidth"
          :min="LIST.min"
          :max="LIST.max"
          @resize="resizeList"
        />
      </div>
      <main class="detail" :class="{ off: layout.isStack && screen === 'list' }" :aria-label="common.shell.detail">
        <template v-if="layout.isStack">
          <ThreadPane :class="{ off: covering }" />
          <ComposePane v-if="covering" class="cover" />
        </template>
        <template v-else>
          <ComposePane v-if="composingNew" />
          <ThreadPane v-else />
        </template>
      </main>
    </div>
    <Transition name="scrim">
      <div v-if="layout.hasDrawer && ui.menuOpen" class="scrim" aria-hidden="true" @click="ui.closeMenu()" />
    </Transition>
    <Transition name="drawer">
      <div v-if="layout.hasDrawer && ui.menuOpen" class="drawer" data-testid="sidebar-drawer">
        <Sidebar />
      </div>
    </Transition>
    <SendConfirmDialog />
    <SettingsDialog />
    <InboxZeroDialog />
    <AddAccountDialog />
    <ToastHost />
  </div>
</template>

<style scoped>
.shell {
  display: flex;
  flex-direction: column;
  height: 100vh;
  overflow: hidden;
  background: var(--bone);
  color: var(--ink);
}

.layout-wide {
  min-width: 960px;
}

/* モバイル幅（Android）では、ソフトキーボードで縮む表示領域の高さに合わせる。 */
.layout-medium,
.layout-stack {
  height: 100dvh;
}

.shell-alerts {
  padding: var(--s-2) var(--s-4);
  border-bottom: 1px solid var(--border);
}

.panes {
  flex: 1;
  display: grid;
  min-height: 0;
}

.col {
  position: relative;
  min-width: 0;
  min-height: 0;
}

.split {
  position: absolute;
  top: 0;
  right: -2px;
  bottom: 0;
}

.off {
  visibility: hidden;
}

.detail {
  min-width: 0;
  min-height: 0;
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

/* .cover（作成画面を詳細に重ねる）の基準。1 画面ずつのレイアウトだけが使う。 */
.layout-stack .detail {
  position: relative;
}

.cover {
  position: absolute;
  inset: 0;
  overflow-y: auto;
}

.scrim {
  position: fixed;
  inset: 0;
  z-index: var(--z-drawer);
  background: var(--scrim);
}

.drawer {
  position: fixed;
  top: 0;
  bottom: 0;
  left: 0;
  z-index: var(--z-drawer);
  width: 78vw;
  max-width: 320px;
}

/* スクラムとドロワーの出入り: フェード＋小さな移動（ガイド M5。reduced-motion ではフェードだけ）。 */
.scrim-enter-active,
.drawer-enter-active {
  transition:
    opacity var(--motion-fade-in) var(--ease-out),
    transform var(--motion-fade-in) var(--ease-out);
}

.scrim-leave-active,
.drawer-leave-active {
  transition:
    opacity var(--motion-fade-out) var(--ease-in),
    transform var(--motion-fade-out) var(--ease-in);
}

.scrim-enter-from,
.scrim-leave-to,
.drawer-enter-from,
.drawer-leave-to {
  opacity: 0;
}

.drawer-enter-from,
.drawer-leave-to {
  transform: translateX(-8px);
}

@media (prefers-reduced-motion: reduce) {
  .drawer-enter-from,
  .drawer-leave-to {
    transform: none;
  }
}

/* 1 画面ずつ: 一覧・スレッド・作成は同じ場所に重ねる。出ていない画面は visibility で隠す（レイアウト・スクロール位置を保ち、フォーカスと読み上げからは外れる）。 */
.layout-stack .col,
.layout-stack .detail {
  grid-area: 1 / 1;
}
</style>
