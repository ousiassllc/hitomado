<script setup lang="ts">
import { User } from '@iconoir/vue'
import { computed } from 'vue'
import { common } from '../../lib/messages'
import { useAccountsStore } from '../../stores/accounts'
import { useInboxStore } from '../../stores/inbox'
import EmptyState from '../ui/EmptyState.vue'
import AccountItem from './AccountItem.vue'

const accounts = useAccountsStore()
const inbox = useInboxStore()

const emit = defineEmits<{ select: [ids: string[]]; reconnect: [id: string] }>()

const selectedIds = computed(() => new Set(inbox.filter.accountIds))
const allSelected = computed(() => selectedIds.value.size === 0)
const totalUnread = computed(() => accounts.accounts.reduce((n, a) => n + (inbox.unreadCounts[a.id] ?? 0), 0))

/** 通常クリック=単一選択（選択済みの単一を押すと「すべて」へ）、Ctrl/Cmd+クリック=複数選択に加える／外す。 */
function onSelect(id: string, multi: boolean): void {
  const cur = [...inbox.filter.accountIds]
  if (multi) {
    const next = cur.includes(id) ? cur.filter((x) => x !== id) : [...cur, id]
    emit('select', next)
  } else if (cur.length === 1 && cur[0] === id) {
    emit('select', [])
  } else {
    emit('select', [id])
  }
}
</script>

<template>
  <nav :aria-label="common.sidebar.label">
    <ul class="list">
      <li class="all" :class="{ selected: allSelected }">
        <button type="button" class="all-btn" :aria-pressed="allSelected" @click="emit('select', [])">
          <span class="all-name">{{ common.sidebar.allAccounts }}</span>
          <span v-if="totalUnread > 0" class="badge" :aria-label="common.sidebar.unread(totalUnread)">{{
            totalUnread > 999 ? '999+' : totalUnread
          }}</span>
        </button>
      </li>
      <AccountItem
        v-for="a in accounts.accounts"
        :key="a.id"
        :account="a"
        :selected="selectedIds.has(a.id)"
        :unread="inbox.unreadCounts[a.id] ?? 0"
        :needs-reauth="accounts.statusOf(a.id) === 'needs_reauth'"
        @select="(multi) => onSelect(a.id, multi)"
        @reconnect="emit('reconnect', a.id)"
      />
    </ul>
    <EmptyState v-if="accounts.accounts.length === 0" compact class="empty">
      <template #icon><User class="icon icon-lg" /></template>
      {{ common.sidebar.noAccounts }}
    </EmptyState>
  </nav>
</template>

<style scoped>
.list {
  display: flex;
  flex-direction: column;
  gap: var(--s-1);
  margin: 0;
  padding: 0;
}

.all {
  list-style: none;
  border-radius: var(--r-sm);
}

.all.selected {
  background: var(--accent-soft);
}

.all-btn {
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
  padding: var(--s-2) var(--s-3);
  border: none;
  background: transparent;
  color: var(--ink);
  font-size: var(--font-size-sm);
  text-align: left;
  cursor: pointer;
  border-radius: var(--r-sm);
}

.all-btn:hover {
  background: var(--accent-soft);
}

.badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  box-sizing: border-box;
  min-width: 20px;
  min-height: 20px;
  padding: 0 var(--s-2);
  border-radius: var(--r-full);
  background: var(--accent);
  color: var(--bone);
  font-size: var(--font-size-xs);
  font-family: var(--font-mono);
  text-align: center;
}

.empty {
  margin: var(--s-3);
}
</style>
