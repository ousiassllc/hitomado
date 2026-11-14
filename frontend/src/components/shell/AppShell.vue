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
import AgentPanel from './AgentPanel.vue'
import AlertStack from './AlertStack.vue'
import BannerStack from './BannerStack.vue'
import BottomTabs from './BottomTabs.vue'
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

/** wide は 3 ペイン、medium はサイドバーの代わりに下 4 タブを使う 2 ペイン、stack は 1 画面ずつ（一覧・スレッド・作成が同じ場所に重なる）。 */
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
/** 下 4 タブを出すか。1 画面ずつのときは、一覧（かエージェントのパネル）のときだけ（スレッド・作成は画面いっぱいに使う）。 */
const showTabs = computed(() => layout.hasTabBar && (!layout.isStack || screen.value === 'list' || ui.agentOpen))
/** スマホでエージェントのパネルが開いているとき、下のペインは操作させない（Tab・読み上げが裏の一覧へ届かないよう inert にする）。 */
const behindAgent = computed(() => ui.agentOpen && layout.hasTabBar)
/** 1 画面ずつのとき、作成・返信は画面全体を覆う ComposePane（下の ThreadPane は残して、戻ったときに読んでいた位置を保つ）。 */
const covering = computed(() => screen.value === 'compose' || screen.value === 'reply')

function onWindowResize(): void {
  total.value = window.innerWidth
  sidebarWidth.value = clampSidebar(sidebarWidth.value, listWidth.value, total.value)
  listWidth.value = clampList(listWidth.value, inlineSidebarWidth(), total.value)
}

/** 一覧の幅の計算に使うサイドバーの幅（サイドバーが無いときは 0）。 */
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
  <div class="shell" :class="[`layout-${layout.mode}`, { 'has-tabs': showTabs }]">
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
      <div class="col" :class="{ off: layout.isStack && screen !== 'list' }" :inert="behindAgent">
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
      <main class="detail" :class="{ off: layout.isStack && screen === 'list' }" :aria-label="common.shell.detail" :inert="behindAgent">
        <template v-if="layout.isStack">
          <ThreadPane :class="{ off: covering }" />
          <ComposePane v-if="covering" class="cover" />
        </template>
        <template v-else>
          <ComposePane v-if="composingNew" />
          <ThreadPane v-else />
        </template>
      </main>
      <!-- エージェントのパネル。本文の上に重ねる（一覧・本文の幅は変えない）。スマホは下タブの上いっぱい。 -->
      <Transition name="agent">
        <AgentPanel v-if="ui.agentOpen" class="agent-panel" />
      </Transition>
    </div>
    <BottomTabs v-if="showTabs" />
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

/* 下タブが出ている間は、トースト（画面下中央）をタブの上へ上げる。 */
.has-tabs {
  --toast-bottom-extra: var(--tabbar-h);
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
  position: relative;
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

/* エージェントのパネル: PC は本文の右端に重ねる。幅は一覧・本文を縮めないよう固定（本文の最小幅 320px を守るため、押し出さず重ねる）。 */
.agent-panel {
  position: absolute;
  top: 0;
  right: 0;
  bottom: 0;
  z-index: var(--z-sticky);
  width: min(344px, 100%);
}

.layout-medium .agent-panel,
.layout-stack .agent-panel {
  left: 0;
  width: auto;
  border-left: none;
}

/* パネルの出入り: フェード＋小さな移動（ガイド M5。reduced-motion ではフェードだけ）。 */
.agent-enter-active {
  transition:
    opacity var(--motion-fade-in) var(--ease-out),
    transform var(--motion-fade-in) var(--ease-out);
}

.agent-leave-active {
  transition:
    opacity var(--motion-fade-out) var(--ease-in),
    transform var(--motion-fade-out) var(--ease-in);
}

.agent-enter-from,
.agent-leave-to {
  opacity: 0;
  transform: translateX(8px);
}

@media (prefers-reduced-motion: reduce) {
  .agent-enter-from,
  .agent-leave-to {
    transform: none;
  }
}

/* 1 画面ずつ: 一覧・スレッド・作成は同じ場所に重ねる。出ていない画面は visibility で隠す（レイアウト・スクロール位置を保ち、フォーカスと読み上げからは外れる）。 */
.layout-stack .col,
.layout-stack .detail {
  grid-area: 1 / 1;
}
</style>
