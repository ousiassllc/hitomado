<script setup lang="ts">
import { computed } from 'vue'
import type { ThreadDetail } from '../../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import { thread as msg } from '../../lib/messages'
import { accountColorVar } from '../../lib/palette'
import { useThreadStore } from '../../stores/thread'
import AccountChip from '../account/AccountChip.vue'
import BaseButton from '../ui/BaseButton.vue'
import BaseMenu from '../ui/BaseMenu.vue'

const props = defineProps<{ detail: ThreadDetail }>()
const store = useThreadStore()

const labelItems = computed(() => {
  const owned = new Set((props.detail.labels ?? []).map((l) => l.id))
  const items = store.accountLabels.map((l) => ({ key: l.id, label: `${owned.has(l.id) ? '✓ ' : ''}${l.name}` }))
  if (items.length > 0) return items
  return [{ key: '', label: store.labelsLoading ? msg.labelsLoading : msg.labelsEmpty, disabled: true }]
})

function onLabel(key: string): void {
  if (key) void store.toggleLabel(key)
}

const subject = computed(() => props.detail.subject || msg.noSubject)
const bandStyle = computed(() => ({ borderTopColor: accountColorVar(props.detail.account.color) }))
</script>

<template>
  <header class="thread-header" :style="bandStyle" data-testid="thread-header">
    <div class="title-row">
      <h2 class="subject" data-testid="thread-subject">{{ subject }}</h2>
      <div class="actions">
        <BaseButton data-testid="act-archive" @click="store.archive()">{{ msg.archive }}</BaseButton>
        <BaseButton data-testid="act-trash" @click="store.trash()">{{ msg.trash }}</BaseButton>
        <BaseMenu :items="labelItems" @select="onLabel">
          <template #trigger>
            <BaseButton
              data-testid="act-label"
              @pointerdown="store.loadLabels()"
              @keydown.enter="store.loadLabels()"
              @keydown.space="store.loadLabels()"
            >{{ msg.label }}</BaseButton>
          </template>
        </BaseMenu>
        <BaseButton data-testid="act-toggle-read" @click="store.toggleRead()">
          {{ store.unread ? msg.markRead : msg.markUnread }}
        </BaseButton>
      </div>
    </div>
    <div class="meta-row">
      <span class="meta-label">{{ msg.receivedAt }}</span>
      <AccountChip :account="detail.account" show-email />
      <span class="count">{{ msg.messageCount((detail.messages ?? []).length) }}</span>
    </div>
  </header>
</template>

<style scoped>
.thread-header {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: var(--space-3) var(--space-4);
  border-top: 4px solid transparent;
  border-bottom: 1px solid var(--color-border);
  background: var(--color-bg);
}

.title-row {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--space-3);
}

.subject {
  margin: 0;
  font-size: var(--font-size-xl);
  overflow-wrap: anywhere;
  color: var(--color-text);
}

.actions {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: var(--space-2);
  flex: none;
}

.meta-row {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  font-size: var(--font-size-sm);
  color: var(--color-text-muted);
}

.count {
  margin-left: auto;
}
</style>
