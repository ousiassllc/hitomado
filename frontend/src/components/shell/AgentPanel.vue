<script setup lang="ts">
import { Xmark } from '@iconoir/vue'
import { nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { nav } from '../../lib/messages'
import { useUiStore } from '../../stores/ui'
import BaseIconButton from '../ui/BaseIconButton.vue'
import EmptyState from '../ui/EmptyState.vue'
import AgentMascot from './AgentMascot.vue'

// エージェントのパネル。PC（3 ペイン）は本文の右から重ねる（一覧の幅も本文の幅も変えない）。スマホは下タブの上いっぱい。
// 中身（エージェントモード）は別 Issue。今は「準備中」だけ。閉じるボタン・Esc・戻る（Android）で閉じる。開いたらパネル自身（名前つきの領域）へフォーカスを移し、閉じたら開いた場所へ戻す。
const ui = useUiStore()
const panel = ref<HTMLElement>()
let opener: HTMLElement | undefined

onMounted(async () => {
  opener = document.activeElement instanceof HTMLElement ? document.activeElement : undefined
  await nextTick()
  panel.value?.focus()
})
onBeforeUnmount(() => {
  if (opener?.isConnected) opener.focus()
})
</script>

<template>
  <aside id="agent-panel" ref="panel" tabindex="-1" class="panel" :aria-label="nav.agent.panelTitle" data-testid="agent-panel" @keydown.esc.stop="ui.closeAgent()">
    <header class="head">
      <AgentMascot :size="32" />
      <h2 class="title">{{ nav.agent.panelTitle }}</h2>
      <BaseIconButton :label="nav.agent.close" data-testid="agent-close" @click="ui.closeAgent()">
        <Xmark class="icon" aria-hidden="true" />
      </BaseIconButton>
    </header>
    <div class="body">
      <EmptyState>
        <template #icon><AgentMascot :size="48" /></template>
        {{ nav.agent.panelBody }}
      </EmptyState>
    </div>
  </aside>
</template>

<style scoped>
.panel {
  display: flex;
  flex-direction: column;
  min-height: 0;
  background: var(--surface);
  border-left: 1px solid var(--border);
}

/* フォーカスは開いたときにパネル自身へ移すだけ（操作する要素ではないので、リングは出さない）。 */
.panel:focus {
  outline: none;
}

.head {
  display: flex;
  align-items: center;
  gap: var(--s-3);
  min-height: var(--topbar-h);
  padding: var(--s-1) var(--s-2) var(--s-1) var(--pad-card-sm);
  border-bottom: 1px solid var(--border);
}

.title {
  flex: 1;
  margin: 0;
  font-size: var(--font-size-base);
  font-weight: 600;
}

.body {
  display: flex;
  flex: 1;
  align-items: center;
  justify-content: center;
  min-height: 0;
  padding: var(--s-6);
  overflow-y: auto;
}
</style>
