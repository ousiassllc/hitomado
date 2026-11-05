<script setup lang="ts">
import { TabsList, TabsRoot, TabsTrigger } from 'reka-ui'
import { INBOX_TABS, type InboxTab } from '../../lib/inboxTabs'
import { common } from '../../lib/messages'

defineProps<{ modelValue: InboxTab }>()
const emit = defineEmits<{ 'update:modelValue': [tab: InboxTab] }>()

function onSelect(v: string | number): void {
  emit('update:modelValue', v as InboxTab)
}
</script>

<template>
  <TabsRoot :model-value="modelValue" activation-mode="automatic" orientation="horizontal" @update:model-value="onSelect">
    <TabsList class="tab-list" :aria-label="common.inboxTabs.label" data-testid="inbox-tabs">
      <TabsTrigger v-for="t in INBOX_TABS" :key="t" :value="t" class="tab" :data-testid="`inbox-tab-${t}`">
        {{ common.inboxTabs[t] }}
      </TabsTrigger>
    </TabsList>
  </TabsRoot>
</template>

<style scoped>
/* 狭い画面（スマホ）では横にスクロールする。 */
.tab-list {
  display: flex;
  gap: var(--s-1);
  padding: 0 var(--s-3);
  border-bottom: 1px solid var(--border);
  overflow-x: auto;
}

.tab {
  flex: none;
  padding: var(--s-2) var(--s-3);
  border: none;
  border-bottom: 2px solid transparent;
  background: transparent;
  color: var(--mute);
  white-space: nowrap;
  cursor: pointer;
}

.tab:hover {
  background: var(--accent-soft);
}

.tab:focus-visible {
  outline: 2px solid var(--focus);
  outline-offset: -2px;
}

.tab[data-state="active"] {
  border-bottom-color: var(--accent);
  color: var(--ink);
}
</style>
