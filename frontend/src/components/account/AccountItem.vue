<script setup lang="ts">
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
      ⚠
    </button>
  </li>
</template>

<style scoped>
.item {
  display: flex;
  align-items: stretch;
  border-radius: var(--radius-sm);
  list-style: none;
}

.item.selected {
  background: var(--color-selected);
}

.main {
  display: flex;
  align-items: center;
  flex: 1;
  min-width: 0;
  gap: var(--space-2);
  padding: var(--space-2) var(--space-2) var(--space-2) 0;
  border: none;
  background: transparent;
  color: var(--color-text);
  text-align: left;
  cursor: pointer;
  border-radius: var(--radius-sm);
}

.main:hover {
  background: var(--color-bg-subtle);
}

.band {
  flex: none;
  align-self: stretch;
  width: 6px;
  border-radius: var(--radius-sm);
  margin-right: var(--space-1);
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
  font-weight: 600;
}

.email {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--color-text-muted);
  font-size: var(--font-size-xs);
}

.badge {
  flex: none;
  min-width: 20px;
  padding: 0 6px;
  border-radius: 10px;
  background: var(--color-accent);
  color: var(--color-accent-text);
  font-size: var(--font-size-xs);
  line-height: 20px;
  text-align: center;
}

.warn {
  flex: none;
  width: 32px;
  border: none;
  background: transparent;
  color: var(--color-danger);
  font-size: var(--font-size-lg);
  cursor: pointer;
}
</style>
