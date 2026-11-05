<script setup lang="ts">
import { useVirtualizer } from '@tanstack/vue-virtual'
import { computed, nextTick, ref, watch } from 'vue'
import type { ThreadRef, ThreadSummary } from '../../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import { keyOf } from '../../lib/inboxItems'
import { common } from '../../lib/messages'
import { useAccountsStore } from '../../stores/accounts'
import BaseButton from '../ui/BaseButton.vue'
import Spinner from '../ui/Spinner.vue'
import ThreadRow from './ThreadRow.vue'

const ROW_HEIGHT = 88
const LOAD_MORE_THRESHOLD = 5

const props = defineProps<{
  items: ThreadSummary[]
  /** 開いているスレッドのキー */
  selectedKey?: string
  /** キーボードカーソルのキー */
  cursorKey?: string
  checkedKeys: string[]
  loading: boolean
  hasMore: boolean
  /** 続きを取得中（自動継続・手動とも） */
  loadingMore: boolean
  emptyText: string
}>()

const emit = defineEmits<{
  open: [ref: ThreadRef]
  toggle: [ref: ThreadRef]
  extend: [ref: ThreadRef]
  loadMore: []
  move: [delta: number, extend: boolean]
}>()

const accounts = useAccountsStore()
const scroller = ref<HTMLElement | null>(null)
const listId = `thread-list-${Math.random().toString(36).slice(2, 8)}`

const virtualizer = useVirtualizer(
  computed(() => ({
    count: props.items.length,
    getScrollElement: () => scroller.value,
    estimateSize: () => ROW_HEIGHT,
    overscan: 8,
    getItemKey: (i: number) => keyOf(props.items[i]),
  })),
)

const rows = computed(() => virtualizer.value.getVirtualItems())
const totalSize = computed(() => virtualizer.value.getTotalSize())
const checkedSet = computed(() => new Set(props.checkedKeys))

function rowId(key: string): string {
  return `${listId}-${key.replace(/[^A-Za-z0-9_-]/g, '_')}`
}
const activeDescendant = computed(() => (props.cursorKey ? rowId(props.cursorKey) : undefined))

function accountOf(t: ThreadSummary) {
  const a = accounts.byId(t.accountId)
  return a ? { ...a, status: accounts.statusOf(a.id) } : undefined
}

// 末尾付近に来たら続きを読み込む
watch(
  () => [rows.value.at(-1)?.index ?? -1, props.items.length, props.hasMore],
  () => {
    const last = rows.value.at(-1)?.index ?? -1
    if (props.hasMore && !props.loading && last >= props.items.length - LOAD_MORE_THRESHOLD) emit('loadMore')
  },
)

// カーソルが画面外に出たらスクロールして見える位置に持ってくる
watch(
  () => props.cursorKey,
  async (key) => {
    if (!key) return
    await nextTick()
    const idx = props.items.findIndex((t) => keyOf(t) === key)
    if (idx >= 0) virtualizer.value.scrollToIndex(idx, { align: 'auto' })
  },
)

function onClick(ev: MouseEvent, t: ThreadSummary): void {
  const ref = { accountId: t.accountId, threadId: t.threadId }
  if (ev.ctrlKey || ev.metaKey) emit('toggle', ref)
  else if (ev.shiftKey) emit('extend', ref)
  else emit('open', ref)
  scroller.value?.focus({ preventScroll: true })
}

function onKeydown(ev: KeyboardEvent): void {
  if (ev.key === 'ArrowDown' || ev.key === 'ArrowUp') {
    ev.preventDefault()
    emit('move', ev.key === 'ArrowDown' ? 1 : -1, ev.shiftKey)
  }
}
</script>

<template>
  <div
    ref="scroller"
    class="list"
    role="listbox"
    tabindex="0"
    aria-multiselectable="true"
    :aria-label="common.list.label"
    :aria-busy="loading"
    :aria-activedescendant="activeDescendant"
    @keydown="onKeydown"
  >
    <div v-if="loading && items.length === 0" class="state"><Spinner /></div>
    <div v-else-if="items.length === 0" class="state">{{ emptyText }}</div>
    <div v-else class="inner" :style="{ height: `${totalSize}px` }">
      <!-- biome-ignore lint/a11y/noStaticElementInteractions lint/a11y/useKeyWithClickEvents: キーボード操作は親の listbox（矢印・j/k・Enter）が担う。この要素は仮想スクロールの位置決め用ラッパー -->
      <div
        v-for="vr in rows"
        :key="String(vr.key)"
        class="slot"
        :style="{ height: `${vr.size}px`, transform: `translateY(${vr.start}px)` }"
        @click="onClick($event, items[vr.index])"
      >
        <ThreadRow
          :id="rowId(keyOf(items[vr.index]))"
          :thread="items[vr.index]"
          :selected="keyOf(items[vr.index]) === selectedKey"
          :checked="checkedSet.has(keyOf(items[vr.index]))"
          :active="keyOf(items[vr.index]) === cursorKey"
          :account="accountOf(items[vr.index])"
        />
      </div>
    </div>
    <!-- 自動継続が上限で止まった場合（未読のみで 0 件等）も、続きを利用者が手動で読める -->
    <div v-if="hasMore && !loading" class="more">
      <BaseButton :loading="loadingMore" @click="emit('loadMore')">{{
        loadingMore ? common.list.loadingMore : common.list.loadMore
      }}</BaseButton>
    </div>
  </div>
</template>

<style scoped>
.list {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  outline: none;
  position: relative;
}

.list:focus-visible {
  outline: 2px solid var(--focus);
  outline-offset: -2px;
}

.inner {
  position: relative;
  width: 100%;
}

.slot {
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
}

.state {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: var(--s-6);
  color: var(--mute);
}

.more {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: var(--s-2);
  padding: var(--s-3);
  color: var(--mute);
  font-size: var(--font-size-sm);
}
</style>
