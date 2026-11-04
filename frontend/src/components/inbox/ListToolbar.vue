<script setup lang="ts">
import { computed } from 'vue'
import { common } from '../../lib/messages'
import { SHORTCUT_KEYS } from '../../lib/shortcuts'
import BaseIconButton from '../ui/BaseIconButton.vue'
import BaseMenu from '../ui/BaseMenu.vue'
import Spinner from '../ui/Spinner.vue'

const props = defineProps<{
  /** 操作対象のスレッド数（0 なら操作ボタンは無効） */
  selectedCount: number
  refreshing: boolean
  searchQuery?: string
  /** 対象が全て同じアカウントのときだけラベルを変更できる */
  labelEnabled: boolean
  /** checked = 対象のスレッドすべてに付いているラベル */
  labels: { id: string; name: string; checked: boolean }[]
  labelsLoading: boolean
  /** 操作ボタン（選択中のスレッドが無い画面）を出さない。1 画面ずつのレイアウトの一覧 */
  compact?: boolean
}>()

const emit = defineEmits<{
  refresh: []
  archive: []
  trash: []
  toggleRead: [read: boolean]
  label: [labelId: string]
  openLabels: []
  backToInbox: []
}>()

const none = computed(() => props.selectedCount === 0)
const menuItems = computed(() => {
  if (props.labelsLoading) return [{ key: '', label: common.toolbar.labelLoading, disabled: true }]
  if (props.labels.length === 0) return [{ key: '', label: common.toolbar.labelEmpty, disabled: true }]
  return props.labels.map((l) => ({ key: l.id, label: `${l.checked ? '✓ ' : ''}${l.name}` }))
})

function onLabelSelect(key: string): void {
  if (key) emit('label', key)
}
</script>

<template>
  <div class="toolbar" role="toolbar" :aria-label="common.list.label">
    <BaseIconButton :label="common.toolbar.refresh" :disabled="refreshing" @click="emit('refresh')">
      <Spinner v-if="refreshing" :size="14" />
      <span v-else aria-hidden="true">⟳</span>
    </BaseIconButton>
    <template v-if="!compact">
      <span class="sep" aria-hidden="true" />
      <BaseIconButton :label="common.toolbar.markRead" :shortcut="SHORTCUT_KEYS.markRead" :disabled="none" @click="emit('toggleRead', true)">
        <span aria-hidden="true">✉</span>
      </BaseIconButton>
      <BaseIconButton :label="common.toolbar.markUnread" :shortcut="SHORTCUT_KEYS.markUnread" :disabled="none" @click="emit('toggleRead', false)">
        <span aria-hidden="true">●</span>
      </BaseIconButton>
      <BaseIconButton :label="common.toolbar.archive" :shortcut="SHORTCUT_KEYS.archive" :disabled="none" @click="emit('archive')">
        <span aria-hidden="true">🗄</span>
      </BaseIconButton>
      <BaseIconButton :label="common.toolbar.trash" :shortcut="SHORTCUT_KEYS.trash" :disabled="none" variant="danger" @click="emit('trash')">
        <span aria-hidden="true">🗑</span>
      </BaseIconButton>
      <BaseMenu :items="menuItems" @select="onLabelSelect">
        <template #trigger>
          <BaseIconButton
            :label="labelEnabled || none ? common.toolbar.label : common.toolbar.labelMultiAccount"
            :disabled="none || !labelEnabled"
            @click="emit('openLabels')"
          >
            <span aria-hidden="true">🏷</span>
          </BaseIconButton>
        </template>
      </BaseMenu>
    </template>
    <span v-if="selectedCount > 1" class="count" role="status">{{ common.toolbar.selected(selectedCount) }}</span>
    <span class="grow" />
    <div v-if="searchQuery" class="mode" role="status">
      <span class="mode-text">{{ common.toolbar.searchMode(searchQuery) }}</span>
      <button type="button" class="back" @click="emit('backToInbox')">{{ common.toolbar.backToInbox }}</button>
    </div>
  </div>
</template>

<style scoped>
.toolbar {
  display: flex;
  align-items: center;
  gap: var(--space-1);
  padding: var(--space-1) var(--space-3);
  border-bottom: 1px solid var(--color-border);
  min-height: 40px;
}

.sep {
  width: 1px;
  height: 20px;
  margin: 0 var(--space-1);
  background: var(--color-border);
}

.grow {
  flex: 1;
}

.count {
  margin-left: var(--space-2);
  color: var(--color-text-muted);
  font-size: var(--font-size-sm);
}

.mode {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  min-width: 0;
  font-size: var(--font-size-sm);
}

.mode-text {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--color-text-muted);
}

.back {
  border: none;
  background: transparent;
  color: var(--color-accent);
  font-size: var(--font-size-sm);
  cursor: pointer;
  text-decoration: underline;
  white-space: nowrap;
}
</style>
