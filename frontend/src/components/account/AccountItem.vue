<script setup lang="ts">
import { WarningTriangle } from '@iconoir/vue'
import { common } from '../../lib/messages'
import BaseIconButton from '../ui/BaseIconButton.vue'
import AccountChip from './AccountChip.vue'

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
      <span class="text">
        <AccountChip class="name" :account="account" />
        <span class="email">{{ account.email }}</span>
      </span>
      <span v-if="unread > 0" class="count" :aria-label="common.sidebar.unread(unread)">{{ unread > 999 ? '999+' : unread }}</span>
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
  padding: var(--s-2) var(--menu-pad-x);
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

.text {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
}

/* 名前は識別色の薄いチップ（一覧と同じ BaseChip の tint。帯は無い）。チップは中身の幅で、メールアドレスの上に左寄せで置く。 */
.name {
  align-self: flex-start;
}

.email {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--mute);
  font-size: var(--font-size-xs);
}

/* --accent-soft 地（選択・ホバー）の上では --mute が AA 4.5:1 に届かないので --ink にする（名前とは文字サイズで区別が残る）。 */
.item.selected .email,
.item.selected .count,
.main:hover .email,
.main:hover .count {
  color: var(--ink);
}

.count {
  flex: none;
  color: var(--mute);
  font-family: var(--font-mono);
  font-size: var(--font-size-xs);
  line-height: var(--line-height-xs);
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
