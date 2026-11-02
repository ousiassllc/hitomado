<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useShortcuts } from '../../composables/useShortcuts'
import { common } from '../../lib/messages'
import { clampList, clampSidebar, LIST, SIDEBAR } from '../../lib/paneWidth'
import { useComposeStore } from '../../stores/compose'
import AddAccountDialog from '../account/AddAccountDialog.vue'
import ComposePane from '../compose/ComposePane.vue'
import SendConfirmDialog from '../compose/SendConfirmDialog.vue'
import ThreadListPane from '../inbox/ThreadListPane.vue'
import SettingsDialog from '../settings/SettingsDialog.vue'
import ThreadPane from '../thread/ThreadPane.vue'
import BannerStack from './BannerStack.vue'
import PaneSplitter from './PaneSplitter.vue'
import Sidebar from './Sidebar.vue'
import ToastHost from './ToastHost.vue'

const compose = useComposeStore()
useShortcuts()

const total = ref(window.innerWidth)
const sidebarWidth = ref<number>(SIDEBAR.initial)
const listWidth = ref<number>(LIST.initial)

const columns = computed(() => `${sidebarWidth.value}px ${listWidth.value}px minmax(0, 1fr)`)
/** 新規作成は右ペイン全面に ComposePane（ThreadPane は隠す）。返信・転送は ThreadPane 内に埋め込まれる。 */
const composingNew = computed(() => compose.session?.mode === 'new')

function onWindowResize(): void {
  total.value = window.innerWidth
  sidebarWidth.value = clampSidebar(sidebarWidth.value, listWidth.value, total.value)
  listWidth.value = clampList(listWidth.value, sidebarWidth.value, total.value)
}

function resizeSidebar(delta: number): void {
  sidebarWidth.value = clampSidebar(sidebarWidth.value + delta, listWidth.value, total.value)
}

function resizeList(delta: number): void {
  listWidth.value = clampList(listWidth.value + delta, sidebarWidth.value, total.value)
}

onMounted(() => window.addEventListener('resize', onWindowResize))
onBeforeUnmount(() => window.removeEventListener('resize', onWindowResize))
</script>

<template>
  <div class="shell">
    <BannerStack />
    <div class="panes" :style="{ gridTemplateColumns: columns }">
      <div class="col">
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
      <div class="col">
        <ThreadListPane />
        <PaneSplitter
          class="split split-2"
          :label="common.shell.listResize"
          :value="listWidth"
          :min="LIST.min"
          :max="LIST.max"
          @resize="resizeList"
        />
      </div>
      <main class="detail" :aria-label="common.shell.detail">
        <ComposePane v-if="composingNew" />
        <ThreadPane v-else />
      </main>
    </div>
    <SendConfirmDialog />
    <SettingsDialog />
    <AddAccountDialog />
    <ToastHost />
  </div>
</template>

<style scoped>
.shell {
  display: flex;
  flex-direction: column;
  height: 100vh;
  min-width: 960px;
  overflow: hidden;
  background: var(--color-bg);
  color: var(--color-text);
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

.detail {
  min-width: 0;
  min-height: 0;
  overflow: hidden;
  display: flex;
  flex-direction: column;
}
</style>
