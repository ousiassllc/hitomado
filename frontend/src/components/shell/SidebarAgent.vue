<script setup lang="ts">
import { computed } from 'vue'
import { agent, nav } from '../../lib/messages'
import { useAgentStore } from '../../stores/agent'
import { useUiStore } from '../../stores/ui'
import AgentMascot from './AgentMascot.vue'

// サイドバーの下（設定の上）のエージェントの入口。押すと右にパネルが開く。一言は、オフ→「オフ」、オン→確認待ちが無ければ「オン」・あれば「確認待ち N件」。
const ui = useUiStore()
const store = useAgentStore()
const sub = computed(() => {
  if (!store.enabled) return agent.entry.off
  return store.pendingCount > 0 ? agent.entry.pending(store.pendingCount) : agent.entry.on
})
</script>

<template>
  <button
    type="button"
    class="agent"
    aria-controls="agent-panel"
    :aria-expanded="ui.agentOpen"
    data-testid="agent-entry"
    @click="ui.agentOpen ? ui.closeAgent() : ui.openAgent()"
  >
    <AgentMascot :size="32" />
    <span class="text">
      <span class="title">{{ nav.agent.label }}</span>
      <span class="sub">{{ sub }}</span>
    </span>
  </button>
</template>

<style scoped>
.agent {
  display: flex;
  align-items: center;
  gap: var(--s-3);
  width: 100%;
  min-height: 48px;
  padding: 0 var(--s-2) 0 var(--s-2);
  border: none;
  border-radius: var(--r-md);
  background: var(--bone);
  color: var(--ink);
  font: inherit;
  text-align: left;
  cursor: pointer;
}

.agent:hover,
.agent[aria-expanded='true'] {
  background: var(--accent-soft);
}

.text {
  display: flex;
  flex-direction: column;
}

.title {
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}

/* --accent-soft 地（ホバー・開いている間）の上では --mute が AA に届かないので --ink にする。 */
.sub {
  color: var(--mute);
  font-size: var(--font-size-xs);
  line-height: var(--line-height-xs);
}

.agent:hover .sub,
.agent[aria-expanded='true'] .sub {
  color: var(--ink);
}
</style>
