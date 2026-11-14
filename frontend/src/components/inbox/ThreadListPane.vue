<script setup lang="ts">
import { Menu } from '@iconoir/vue'
import { computed, ref } from 'vue'
import type { Label } from '../../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import * as InboxService from '../../../bindings/github.com/ousiassllc/hitomado/internal/app/inboxservice'
import { call, list, parseBindingError } from '../../lib/api'
import { keyOf } from '../../lib/inboxItems'
import { common, drafts as draftsMsg } from '../../lib/messages'
import { useComposeStore } from '../../stores/compose'
import { useDraftsStore } from '../../stores/drafts'
import { useInboxStore } from '../../stores/inbox'
import { useLayoutStore } from '../../stores/layout'
import { useUiStore } from '../../stores/ui'
import DraftList from '../drafts/DraftList.vue'
import BaseButton from '../ui/BaseButton.vue'
import BaseIconButton from '../ui/BaseIconButton.vue'
import InboxTabs from './InboxTabs.vue'
import ListToolbar from './ListToolbar.vue'
import SearchBar from './SearchBar.vue'
import ThreadList from './ThreadList.vue'

const inbox = useInboxStore()
const ui = useUiStore()
const layout = useLayoutStore()
const compose = useComposeStore()
const drafts = useDraftsStore()

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
  if (inbox.filter.unreadOnly || inbox.tab === 'new') return common.list.emptyUnread
  return inbox.tab === 'primary' || inbox.tab === 'social' || inbox.tab === 'promotions'
    ? common.list.emptyCategory
    : common.list.empty
})

async function loadLabels(): Promise<void> {
  const [accountId] = targetAccounts.value
  if (targetAccounts.value.size !== 1 || !accountId) return
  labelsLoading.value = true
  try {
    labels.value = list(await call(InboxService.ListLabels(accountId)))
  } catch (e) {
    labels.value = []
    ui.pushAlert(parseBindingError(e).message)
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
  <section class="pane" :aria-label="drafts.active ? draftsMsg.label : common.list.label">
    <!-- サイドバーをドロワーにするレイアウト（2 ペイン・1 画面ずつ）の上部バー -->
    <header v-if="layout.hasDrawer" class="head">
      <BaseIconButton :label="common.shell.menu" data-testid="open-menu" @click="ui.openMenu()">
        <Menu class="icon" aria-hidden="true" />
      </BaseIconButton>
      <span class="app-name">{{ common.appName }}</span>
      <BaseButton size="sm" variant="primary" data-testid="head-compose" @click="compose.open({ mode: 'new' })">{{ common.sidebar.compose }}</BaseButton>
    </header>
    <DraftList v-if="drafts.active" />
    <template v-else>
      <SearchBar :active="inbox.mode === 'search'" :query="inbox.searchQuery" @search="inbox.runSearch" @clear="inbox.clearSearch" />
      <InboxTabs v-if="inbox.mode === 'inbox'" :model-value="inbox.tab" @update:model-value="inbox.setTab" />
      <ListToolbar
        :selected-count="targets.length"
        :refreshing="inbox.refreshing"
        :searching="inbox.mode === 'search'"
        :compact="layout.isStack"
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
        :cursor-key="layout.isStack ? undefined : inbox.cursorKey"
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
    </template>
  </section>
</template>

<style scoped>
.pane {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-width: 0;
  min-height: 0;
  background: var(--surface);
  border-right: 1px solid var(--border);
}

.head {
  display: flex;
  align-items: center;
  gap: var(--s-2);
  min-height: var(--topbar-h);
  padding: var(--s-1) var(--s-3);
  border-bottom: 1px solid var(--border);
}

.app-name {
  flex: 1;
  font-size: var(--font-size-base);
}
</style>
