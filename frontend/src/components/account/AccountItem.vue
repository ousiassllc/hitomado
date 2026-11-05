<script setup lang="ts">
import { WarningTriangle } from '@iconoir/vue'
import { common } from '../../lib/messages'
import { accountColorVar } from '../../lib/palette'

defineProps<{
  account: { id: string; displayName: string; email: string; color: string }
  selected: boolean
  unread: number
  needsReauth: boolean
}>()

const emit = defineEmits<{ select: [multi: boolean]; reconnect: [] }>()
</script>

<template>
  <li class="item" :class="{ selected }">
    <button
      type="button"
      class="main"
      :aria-pressed="selected"
      :title="account.email"
      @click="emit('select', $event.ctrlKey || $event.metaKey)"
    >
      <span class="band" :style="{ background: accountColorVar(account.color) }" aria-hidden="true" />
      <span class="text">
        <span class="name">{{ account.displayName }}</span>
        <span class="email">{{ account.email }}</span>
      </span>
      <span v-if="unread > 0" class="badge" :aria-label="common.sidebar.unread(unread)">{{ unread > 999 ? '999+' : unread }}</span>
    </button>
    <button
      v-if="needsReauth"
      type="button"
      class="warn"
      :aria-label="`${account.displayName}: ${common.sidebar.needsReauth}（${common.reconnect}）`"
      :title="common.sidebar.needsReauth"
      @click="emit('reconnect')"
    >
      <WarningTriangle class="icon icon-sm" aria-hidden="true" />
    </button>
  </li>
</template>

<style scoped>
.item {
  display: flex;
  align-items: stretch;
  border-radius: var(--r-sm);
  list-style: none;
}

.item.selected {
  background: var(--accent-soft);
}

.main {
  display: flex;
  align-items: center;
  flex: 1;
  min-width: 0;
  gap: var(--s-2);
  padding: var(--s-2) var(--s-2) var(--s-2) 0;
  border: none;
  background: transparent;
  color: var(--ink);
  text-align: left;
  cursor: pointer;
  border-radius: var(--r-sm);
}

.main:hover {
  background: var(--accent-soft);
}

.band {
  flex: none;
  align-self: stretch;
  width: 6px;
  border-radius: var(--r-sm);
  margin-right: var(--s-1);
}

.text {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
}

.name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: var(--font-size-sm);
}

.email {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--mute);
  font-size: var(--font-size-xs);
}

.badge {
  flex: none;
  min-width: 20px;
  padding: 0 var(--s-2);
  border-radius: var(--r-full);
  background: var(--accent);
  color: var(--bone);
  font-size: var(--font-size-xs);
  font-family: var(--font-mono);
  line-height: 20px;
  text-align: center;
}

.warn {
  flex: none;
  width: 32px;
  border: none;
  background: transparent;
  color: var(--danger);
  font-size: var(--font-size-base);
  cursor: pointer;
}
</style>
