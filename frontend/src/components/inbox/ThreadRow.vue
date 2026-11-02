<script setup lang="ts">
import { computed } from 'vue'
import type { ThreadSummary } from '../../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import { formatDate } from '../../lib/format'
import { common } from '../../lib/messages'
import { accountColorVar } from '../../lib/palette'
import AccountChip from '../account/AccountChip.vue'
import LabelChip from './LabelChip.vue'

const props = defineProps<{
  thread: ThreadSummary
  /** 開いている（スレッドペインに表示中の）行 */
  selected: boolean
  /** 複数選択に含まれる行 */
  checked?: boolean
  /** キーボードカーソルがある行 */
  active?: boolean
  account?: { displayName: string; email: string; color: string; status?: string }
  /** 行の DOM id（aria-activedescendant 用） */
  id?: string
}>()

const sender = computed(() => props.thread.lastFrom.name || props.thread.lastFrom.email || common.list.noSender)
const faded = computed(() => props.account?.status === 'needs_reauth')
const labels = computed(() => props.thread.labels ?? [])
const label = computed(
  () =>
    `${props.thread.isUnread ? `${common.list.unreadMark}、` : ''}${props.account?.displayName ?? ''} ${sender.value}、${
      props.thread.subject || common.list.noSubject
    }`,
)
</script>

<template>
  <div
    :id="id"
    class="row"
    :class="{
      unread: thread.isUnread,
      selected,
      checked,
      active,
      faded,
    }"
    role="option"
    tabindex="-1"
    :aria-selected="selected || checked || false"
    :aria-label="label"
    :title="faded ? common.list.needsReauthRow : undefined"
  >
    <span class="band" :style="{ background: account ? accountColorVar(account.color) : 'var(--color-border)' }" aria-hidden="true" />
    <div class="main">
      <div class="line1">
        <AccountChip v-if="account" :account="account" size="sm" />
        <span class="sender">{{ sender }}</span>
        <span v-if="thread.messageCount > 1" class="count" :title="common.list.messageCount(thread.messageCount)">{{
          thread.messageCount
        }}</span>
        <span v-if="thread.hasAttachment" class="clip" role="img" :aria-label="common.list.attachment">📎</span>
        <time class="date">{{ formatDate(thread.lastMessageAt) }}</time>
      </div>
      <div class="line2">
        <span v-if="thread.isUnread" class="dot" aria-hidden="true" />
        <span class="subject">{{ thread.subject || common.list.noSubject }}</span>
        <span class="snippet">{{ thread.snippet }}</span>
      </div>
      <div v-if="labels.length > 0" class="labels">
        <LabelChip v-for="l in labels" :key="l.id" :label="l" />
      </div>
    </div>
  </div>
</template>

<style scoped>
.row {
  display: flex;
  align-items: stretch;
  height: 100%;
  border-bottom: 1px solid var(--color-border);
  background: var(--color-bg);
  cursor: pointer;
  overflow: hidden;
}

.row.unread {
  background: var(--color-unread-bg);
}

.row:hover {
  background: var(--color-bg-subtle);
}

.row.selected,
.row.checked {
  background: var(--color-selected);
}

.row.active {
  outline: 2px solid var(--color-focus);
  outline-offset: -2px;
}

.band {
  flex: none;
  width: 6px;
}

.main {
  flex: 1;
  min-width: 0;
  padding: var(--space-1) var(--space-3);
  display: flex;
  flex-direction: column;
  justify-content: center;
  gap: 2px;
}

.line1,
.line2,
.labels {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  min-width: 0;
}

.labels {
  overflow: hidden;
}

.sender {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: var(--font-size-sm);
}

.count {
  flex: none;
  color: var(--color-text-muted);
  font-size: var(--font-size-xs);
}

.clip {
  flex: none;
  font-size: var(--font-size-xs);
}

.date {
  flex: none;
  color: var(--color-text-muted);
  font-size: var(--font-size-xs);
}

.dot {
  flex: none;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--color-accent);
}

.subject {
  flex: none;
  max-width: 55%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: var(--font-size-sm);
}

.snippet {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--color-text-muted);
  font-size: var(--font-size-sm);
}

.unread .sender,
.unread .subject {
  font-weight: 700;
}

.row.faded .sender,
.row.faded .subject,
.row.faded .snippet,
.row.faded .date,
.row.faded .count {
  color: var(--color-text-faded);
  font-weight: 400;
}
</style>
