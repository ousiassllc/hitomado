<script setup lang="ts">
import { WarningTriangle } from '@iconoir/vue'
import { common } from '../../lib/messages'
import { accountColorVar } from '../../lib/palette'
import BaseIconButton from '../ui/BaseIconButton.vue'

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
    <BaseIconButton
      v-if="needsReauth"
      class="warn"
      variant="ghost"
      :label="`${account.displayName}: ${common.sidebar.needsReauth}（${common.reconnect}）`"
      @click="emit('reconnect')"
    >
      <WarningTriangle class="icon icon-sm warn-icon" aria-hidden="true" />
    </BaseIconButton>
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
  padding: var(--s-2) var(--menu-pad-x) var(--s-2) 0;
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
  display: inline-flex;
  align-items: center;
  justify-content: center;
  box-sizing: border-box;
  min-width: 20px;
  min-height: 20px;
  padding: 0 var(--s-2);
  border-radius: var(--r-full);
  background: var(--accent);
  color: var(--bone);
  font-size: var(--font-size-xs);
  line-height: var(--line-height-xs);
  font-family: var(--font-mono);
  text-align: center;
}

/* 要再接続は他の箇所と同じく警告色（--danger は使わない）。 */
.warn-icon {
  color: var(--warning);
}

/* BaseIconButton の属性はボタンへ渡るので、scoped の属性が付かない。:deep で当てる。 */
.item :deep(.warn) {
  flex: none;
  align-self: center;
}
</style>
