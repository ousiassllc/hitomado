<script setup lang="ts">
import { nav } from '../../lib/messages'
import { useUiStore } from '../../stores/ui'
import AgentMascot from './AgentMascot.vue'

// サイドバーの下（設定の上）のエージェントの入口。押すと右にパネルが開く。中身（エージェントモード）は別 Issue なので、
// 今は「準備中」とだけ出す（確認待ちの件数は中身ができてから出す）。
const ui = useUiStore()
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
      <span class="sub">{{ nav.agent.preparing }}</span>
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
