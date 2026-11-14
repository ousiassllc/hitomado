<script setup lang="ts">
import { Xmark } from '@iconoir/vue'
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { agent, nav } from '../../lib/messages'
import { useAgentStore } from '../../stores/agent'
import { useUiStore } from '../../stores/ui'
import AgentItemCard from '../agent/AgentItemCard.vue'
import AgentSection from '../agent/AgentSection.vue'
import BaseButton from '../ui/BaseButton.vue'
import BaseIconButton from '../ui/BaseIconButton.vue'
import EmptyState from '../ui/EmptyState.vue'
import AgentMascot from './AgentMascot.vue'

// エージェントのパネル。PC（3 ペイン）は本文の右から重ねる（一覧の幅も本文の幅も変えない）。スマホは下タブの上いっぱい。
// 中身（エージェントモード）: オフのときは説明と設定への案内、オンのときは質問・確認待ち・失敗・あとで・返信不要の一覧。確認済みは出さない。
// 閉じるボタン・Esc・戻る（Android）で閉じる。開いたらパネル自身（名前つきの領域）へフォーカスを移し、閉じたら開いた場所へ戻す。
const ui = useUiStore()
const store = useAgentStore()
const msg = agent.panel

const itemsOf = (status: string) => store.items.filter((i) => i.status === status)
const sections = computed(() => [
  { status: 'asking', title: msg.sections.asking, collapsible: false },
  { status: 'ready', title: msg.sections.ready, collapsible: false },
  { status: 'failed', title: msg.sections.failed, collapsible: false },
  { status: 'later', title: msg.sections.later, collapsible: true },
  { status: 'skipped', title: msg.sections.skipped, collapsible: true },
])
const empty = computed(() => sections.value.every((s) => itemsOf(s.status).length === 0))
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
    <!-- 件数が変わったことを読み上げる（見えない。画面の更新は agent:updated で起きるので、利用者の操作とは限らない）。 -->
    <p v-if="store.enabled" class="sr-only" role="status" aria-live="polite" data-testid="agent-live">{{ agent.entry.pending(store.pendingCount) }}</p>
    <div class="body">
      <EmptyState v-if="!store.enabled" data-testid="agent-off">
        <template #icon><AgentMascot :size="48" /></template>
        <p class="state-title">{{ msg.off.title }}</p>
        <p>{{ msg.off.body }}</p>
        <p>{{ msg.off.note }}</p>
        <template #action>
          <BaseButton variant="primary" data-testid="agent-open-settings" @click="ui.openSettings('agent')">
            {{ msg.off.openSettings }}
          </BaseButton>
        </template>
      </EmptyState>
      <EmptyState v-else-if="empty" data-testid="agent-empty">
        <template #icon><AgentMascot :size="48" /></template>
        <p class="state-title">{{ msg.empty.title }}</p>
        <p>{{ msg.empty.body }}</p>
      </EmptyState>
      <template v-else>
        <AgentSection
          v-for="sec in sections"
          :key="sec.status"
          :title="sec.title"
          :count="itemsOf(sec.status).length"
          :collapsible="sec.collapsible"
          :data-testid="`agent-section-${sec.status}`"
        >
          <AgentItemCard v-for="item in itemsOf(sec.status)" :key="item.id" :item="item" :heading="sec.collapsible ? 'h3' : 'h4'" />
        </AgentSection>
      </template>
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
  flex-direction: column;
  gap: var(--s-4);
  min-height: 0;
  padding: var(--pad-card-sm);
  overflow-y: auto;
}

.sr-only {
  position: absolute;
  width: 1px;
  height: 1px;
  overflow: hidden;
  clip: rect(0 0 0 0);
  white-space: nowrap;
}

.state-title {
  color: var(--ink);
  font-size: var(--font-size-base);
  line-height: var(--line-height-base);
  font-weight: 600;
}
</style>
