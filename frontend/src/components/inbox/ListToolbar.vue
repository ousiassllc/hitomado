<script setup lang="ts">
import { Archive, Label, Mail, MailOpen, Refresh, Trash } from '@iconoir/vue'
import { computed } from 'vue'
import { common } from '../../lib/messages'
import { SHORTCUT_KEYS } from '../../lib/shortcuts'
import BaseIconButton from '../ui/BaseIconButton.vue'
import BaseMenu from '../ui/BaseMenu.vue'
import BaseSwitch from '../ui/BaseSwitch.vue'
import Spinner from '../ui/Spinner.vue'

const props = defineProps<{
  /** 操作対象のスレッド数。一括操作のボタンは 2 件以上のときだけ出す（1 件はスレッドのヘッダーの操作で足りる） */
  selectedCount: number
  refreshing: boolean
  /** 検索結果モード中（「受信箱に戻る」を出す。未読のみは検索に効かないので出さない） */
  searching?: boolean
  /** 未読のみ（一覧の上のトグル。サイドバーにあったものをここへ移した） */
  unreadOnly: boolean
  /** 対象が全て同じアカウントのときだけラベルを変更できる */
  labelEnabled: boolean
  /** checked = 対象のスレッドすべてに付いているラベル */
  labels: { id: string; name: string; checked: boolean }[]
  labelsLoading: boolean
  /** 一括操作のボタン（選択中のスレッドが無い画面）を出さない。1 画面ずつのレイアウトの一覧 */
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
  'update:unreadOnly': [value: boolean]
}>()

const bulk = computed(() => !props.compact && props.selectedCount > 1)
const menuItems = computed(() => {
  if (props.labelsLoading) return [{ key: '', label: common.toolbar.labelLoading, disabled: true }]
  if (props.labels.length === 0) return [{ key: '', label: common.toolbar.labelEmpty, disabled: true }]
  return props.labels.map((l) => ({ key: l.id, label: l.name, checked: l.checked }))
})

/** メニューが開いた経路によらず、開いた時点でラベルを取得させる。 */
function onMenuOpen(open: boolean): void {
  if (open) emit('openLabels')
}

function onLabelSelect(key: string): void {
  if (key) emit('label', key)
}
</script>

<template>
  <div class="toolbar" role="toolbar" :aria-label="common.list.label">
    <BaseIconButton :label="common.toolbar.refresh" :loading="refreshing" @click="emit('refresh')">
      <Spinner v-if="refreshing" />
      <Refresh v-else class="icon" aria-hidden="true" />
    </BaseIconButton>
    <template v-if="bulk">
      <span class="sep" aria-hidden="true" />
      <BaseIconButton :label="common.toolbar.markRead" :shortcut="SHORTCUT_KEYS.markRead" @click="emit('toggleRead', true)">
        <MailOpen class="icon" aria-hidden="true" />
      </BaseIconButton>
      <BaseIconButton :label="common.toolbar.markUnread" :shortcut="SHORTCUT_KEYS.markUnread" @click="emit('toggleRead', false)">
        <Mail class="icon" aria-hidden="true" />
      </BaseIconButton>
      <BaseIconButton :label="common.toolbar.archive" :shortcut="SHORTCUT_KEYS.archive" @click="emit('archive')">
        <Archive class="icon" aria-hidden="true" />
      </BaseIconButton>
      <BaseIconButton :label="common.toolbar.trash" :shortcut="SHORTCUT_KEYS.trash" @click="emit('trash')">
        <Trash class="icon" aria-hidden="true" />
      </BaseIconButton>
      <BaseMenu :items="menuItems" @select="onLabelSelect" @open="onMenuOpen">
        <template #trigger>
          <BaseIconButton
            :label="labelEnabled ? common.toolbar.label : common.toolbar.labelMultiAccount"
            :disabled="!labelEnabled"
          >
            <Label class="icon" aria-hidden="true" />
          </BaseIconButton>
        </template>
      </BaseMenu>
    </template>
    <span v-if="selectedCount > 1" class="count" role="status">{{ common.toolbar.selected(selectedCount) }}</span>
    <span class="grow" />
    <div v-if="searching" class="mode">
      <button type="button" class="back" @click="emit('backToInbox')">{{ common.toolbar.backToInbox }}</button>
    </div>
    <div v-else class="unread-only">
      <span>{{ common.filter.unreadOnly }}</span>
      <BaseSwitch :model-value="unreadOnly" :ariaLabel="common.filter.unreadOnly" @update:model-value="(v) => emit('update:unreadOnly', v)" />
    </div>
  </div>
</template>

<style scoped>
.toolbar {
  display: flex;
  align-items: center;
  gap: var(--s-1);
  padding: var(--s-1) var(--s-3);
  border-bottom: 1px solid var(--border);
  min-height: var(--control-h);
}

.sep {
  width: 1px;
  height: var(--icon-md);
  margin: 0 var(--s-1);
  background: var(--border);
}

.grow {
  flex: 1;
}

.count {
  margin-left: var(--s-2);
  color: var(--mute);
  font-size: var(--font-size-sm);
}

.mode {
  display: flex;
  align-items: center;
  gap: var(--s-2);
  min-width: 0;
  font-size: var(--font-size-sm);
}

.unread-only {
  display: flex;
  align-items: center;
  gap: var(--s-2);
  font-size: var(--font-size-sm);
  white-space: nowrap;
}

.back {
  border: none;
  background: transparent;
  color: var(--ink);
  font-size: var(--font-size-sm);
  cursor: pointer;
  text-decoration: underline;
  text-decoration-color: var(--mute);
  text-underline-offset: 2px;
  white-space: nowrap;
}
</style>
