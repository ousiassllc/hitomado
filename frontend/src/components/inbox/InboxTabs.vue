<script setup lang="ts">
import { TabsList, TabsRoot, TabsTrigger } from 'reka-ui'
import { INBOX_TABS, type InboxTab } from '../../lib/inboxTabs'
import { common } from '../../lib/messages'
import { vOverlayScroll } from '../../lib/overlayScroll'
import InboxZeroButton from './InboxZeroButton.vue'

defineProps<{ modelValue: InboxTab }>()
const emit = defineEmits<{ 'update:modelValue': [tab: InboxTab] }>()

function onSelect(v: string | number): void {
  emit('update:modelValue', v as InboxTab)
}
</script>

<template>
  <!-- Inbox Zero ボタンはタブの横スクロールの外（左端）に置く。タブが横にスクロールしても隠れず、タブとぶつからない。 -->
  <div class="tabs-row">
    <InboxZeroButton />
    <TabsRoot class="tabs-root" :model-value="modelValue" activation-mode="automatic" orientation="horizontal" @update:model-value="onSelect">
      <TabsList v-overlay-scroll class="tab-list" :aria-label="common.inboxTabs.label" data-testid="inbox-tabs">
        <TabsTrigger v-for="t in INBOX_TABS" :key="t" :value="t" class="tab" :data-testid="`inbox-tab-${t}`">
          {{ common.inboxTabs[t] }}
        </TabsTrigger>
      </TabsList>
    </TabsRoot>
  </div>
</template>

<style scoped>
.tabs-row {
  display: flex;
  align-items: center;
  gap: var(--s-1);
  padding-left: var(--s-3);
  border-bottom: 1px solid var(--border);
}

.tabs-root {
  flex: 1;
  min-width: 0;
}

/* 狭い画面（スマホ）では横にスクロールする（ボタンは外にあるので一緒に流れない）。 */
.tab-list {
  display: flex;
  gap: var(--s-1);
  padding: 0 var(--s-3) 0 var(--s-1);
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
  /* --accent-soft 地の上では --mute が AA 4.5:1 に届かないので --ink にする（選択タブは下線 --accent で区別が残る）。 */
  color: var(--ink);
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
