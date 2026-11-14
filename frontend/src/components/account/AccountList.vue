<script setup lang="ts">
import { Plus, User } from '@iconoir/vue'
import { useAccountFilter } from '../../composables/useAccountFilter'
import { common, nav } from '../../lib/messages'
import { useAccountsStore } from '../../stores/accounts'
import { useInboxStore } from '../../stores/inbox'
import EmptyState from '../ui/EmptyState.vue'
import AccountItem from './AccountItem.vue'

// サイドバーの「アカウント」。見る場所（受信箱・下書き…）とは別の軸の絞り込みで、組み合わせて効く。
// 数（未読）は受信箱の行に付けるので、ここは各アカウントの行に控えめに添えるだけ。
const accounts = useAccountsStore()
const inbox = useInboxStore()
const { selectedIds, allSelected, toggle, selectAll } = useAccountFilter()

const emit = defineEmits<{ reconnect: [id: string]; add: [] }>()
</script>

<template>
  <section class="accounts" aria-labelledby="sidebar-accounts-heading">
    <h2 id="sidebar-accounts-heading" class="heading">{{ nav.accounts.heading }}</h2>
    <ul class="list">
      <li class="all" :class="{ selected: allSelected }">
        <button type="button" class="all-btn" :aria-pressed="allSelected" @click="selectAll">
          <span class="all-name">{{ nav.accounts.all }}</span>
        </button>
      </li>
      <AccountItem
        v-for="a in accounts.accounts"
        :key="a.id"
        :account="a"
        :selected="selectedIds.has(a.id)"
        :unread="inbox.unreadCounts[a.id] ?? 0"
        :needs-reauth="accounts.statusOf(a.id) === 'needs_reauth'"
        @select="(multi) => toggle(a.id, multi)"
        @reconnect="emit('reconnect', a.id)"
      />
    </ul>
    <EmptyState v-if="accounts.accounts.length === 0" compact class="empty">
      <template #icon><User class="icon icon-lg" /></template>
      {{ common.sidebar.noAccounts }}
    </EmptyState>
    <button type="button" class="add" @click="emit('add')">
      <Plus class="icon icon-sm" aria-hidden="true" />{{ common.sidebar.addAccount }}
    </button>
  </section>
</template>

<style scoped>
.heading {
  margin: 0;
  padding: var(--s-4) var(--menu-pad-x) var(--s-1);
  color: var(--mute);
  font-size: var(--font-size-xs);
  font-weight: 400;
  line-height: var(--line-height-xs);
}

.list {
  display: flex;
  flex-direction: column;
  gap: var(--gap-menu);
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
  width: 100%;
  min-height: var(--menu-item-h);
  padding: 0 var(--menu-pad-x);
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

.empty {
  margin: var(--s-3);
}

/* 「アカウントを追加」はアカウントの並びの終わりに置く（一番下は設定の場所）。 */
.add {
  display: flex;
  align-items: center;
  gap: var(--s-2);
  width: 100%;
  min-height: var(--menu-item-h);
  margin-top: var(--gap-menu);
  padding: 0 var(--menu-pad-x);
  border: none;
  border-radius: var(--r-sm);
  background: transparent;
  color: var(--ink);
  font: inherit;
  font-size: var(--font-size-sm);
  text-align: left;
  cursor: pointer;
}

.add:hover {
  background: var(--accent-soft);
}
</style>
