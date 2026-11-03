<script setup lang="ts">
import { computed, ref } from 'vue'
import type { Label } from '../../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import * as InboxService from '../../../bindings/github.com/ousiassllc/hitomado/internal/app/inboxservice'
import { call, list, parseBindingError } from '../../lib/api'
import { keyOf } from '../../lib/inboxItems'
import { common } from '../../lib/messages'
import { useInboxStore } from '../../stores/inbox'
import { useUiStore } from '../../stores/ui'
import ListToolbar from './ListToolbar.vue'
import SearchBar from './SearchBar.vue'
import ThreadList from './ThreadList.vue'

const inbox = useInboxStore()
const ui = useUiStore()

const labels = ref<Label[]>([])
const labelsLoading = ref(false)

const targets = computed(() => inbox.targetRefs)
const targetAccounts = computed(() => new Set(targets.value.map((r) => r.accountId)))
const labelEnabled = computed(() => targetAccounts.value.size === 1)

const labelItems = computed(() => {
  const keys = new Set(targets.value.map((r) => `${r.accountId}:${r.threadId}`))
  const rows = inbox.items.filter((t) => keys.has(keyOf(t)))
  return labels.value.map((l) => ({
    id: l.id,
    name: l.name,
    checked: rows.length > 0 && rows.every((t) => list(t.labels).some((x) => x.id === l.id)),
  }))
})

const emptyText = computed(() => {
  if (inbox.mode === 'search') return common.list.emptySearch
  return inbox.filter.unreadOnly ? common.list.emptyUnread : common.list.empty
})

async function loadLabels(): Promise<void> {
  const [accountId] = targetAccounts.value
  if (targetAccounts.value.size !== 1 || !accountId) return
  labelsLoading.value = true
  try {
    labels.value = list(await call(InboxService.ListLabels(accountId)))
  } catch (e) {
    labels.value = []
    ui.pushToast('error', parseBindingError(e).message)
  } finally {
    labelsLoading.value = false
  }
}

async function toggleLabel(labelId: string): Promise<void> {
  const item = labelItems.value.find((l) => l.id === labelId)
  if (!item) return
  const ok = await inbox.modifyLabel(targets.value, labelId, !item.checked)
  if (ok) ui.pushToast('success', common.op.labelUpdated)
}

</script>

<template>
  <section class="pane" :aria-label="common.list.label">
    <SearchBar :active="inbox.mode === 'search'" :query="inbox.searchQuery" @search="inbox.runSearch" @clear="inbox.clearSearch" />
    <ListToolbar
      :selected-count="targets.length"
      :refreshing="inbox.refreshing"
      :search-query="inbox.mode === 'search' ? inbox.searchQuery : undefined"
      :label-enabled="labelEnabled"
      :labels="labelItems"
      :labels-loading="labelsLoading"
      @refresh="inbox.refresh"
      @archive="inbox.archive(targets)"
      @trash="inbox.trash(targets)"
      @toggle-read="(read) => inbox.markRead(targets, read)"
      @open-labels="loadLabels"
      @label="toggleLabel"
      @back-to-inbox="inbox.clearSearch"
    />
    <ThreadList
      :items="inbox.items"
      :selected-key="inbox.selectedKey"
      :cursor-key="inbox.cursorKey"
      :checked-keys="inbox.checkedKeys"
      :loading="inbox.loading"
      :has-more="inbox.hasMore"
      :loading-more="inbox.loadingMore"
      :empty-text="emptyText"
      @open="inbox.select"
      @toggle="inbox.toggleChecked"
      @extend="inbox.extendChecked"
      @load-more="inbox.loadMore"
      @move="inbox.moveCursor"
    />
  </section>
</template>

<style scoped>
.pane {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-width: 0;
  min-height: 0;
  background: var(--color-bg);
  border-right: 1px solid var(--color-border);
}
</style>
