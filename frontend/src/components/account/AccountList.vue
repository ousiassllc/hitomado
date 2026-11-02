<script setup lang="ts">
import { computed } from 'vue'
import { common } from '../../lib/messages'
import { useAccountsStore } from '../../stores/accounts'
import { useInboxStore } from '../../stores/inbox'
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
    <p v-if="accounts.accounts.length === 0" class="empty">{{ common.sidebar.noAccounts }}</p>
  </nav>
</template>

<style scoped>
.list {
  display: flex;
  flex-direction: column;
  gap: 2px;
  margin: 0;
  padding: 0;
}

.all {
  list-style: none;
  border-radius: var(--radius-sm);
}

.all.selected {
  background: var(--color-selected);
}

.all-btn {
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
  padding: var(--space-2) var(--space-3);
  border: none;
  background: transparent;
  color: var(--color-text);
  font-size: var(--font-size-sm);
  font-weight: 600;
  text-align: left;
  cursor: pointer;
  border-radius: var(--radius-sm);
}

.all-btn:hover {
  background: var(--color-bg-subtle);
}

.badge {
  min-width: 20px;
  padding: 0 6px;
  border-radius: 10px;
  background: var(--color-accent);
  color: var(--color-accent-text);
  font-size: var(--font-size-xs);
  line-height: 20px;
  text-align: center;
}

.empty {
  margin: var(--space-3);
  color: var(--color-text-muted);
  font-size: var(--font-size-sm);
}
</style>
