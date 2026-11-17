<script setup lang="ts">
import { WarningTriangle } from '@iconoir/vue'
import { computed } from 'vue'
import { common } from '../../../lib/messages'
import { useAccountsStore } from '../../../stores/accounts'
import type { Failure } from '../../../stores/inboxFailures'
import BaseButton from '../../ui/BaseButton.vue'

// 一部のアカウントだけ取得に失敗したときの、一覧の上の案内（#392）。見出し・「アカウント名：理由」・本文の下に「再試行」。
// 色は警告色にせず、選択行と同じ無彩色の面（--accent-soft）・ミュートのアイコン。全アカウントが失敗したときは出さず、空表示に「再試行」を置く。
// 要再接続・オフラインは BannerStack が出すので、呼び出し側が除いて渡す。
const props = defineProps<{ failures: Failure[]; busy?: boolean }>()
defineEmits<{ retry: [] }>()

const accounts = useAccountsStore()
const nameOf = (id: string) => accounts.byId(id)?.displayName || common.list.unknownAccount

// 同じ理由（全アカウントが利用制限に当たったときなど）は 1 行にまとめ、アカウント名を並べる。
const rows = computed(() => {
  const byReason = new Map<string, string[]>()
  for (const f of props.failures) byReason.set(f.message, [...(byReason.get(f.message) ?? []), nameOf(f.accountId)])
  return [...byReason].map(([reason, names]) => ({ reason, text: common.list.failedAccount(names.join('、'), reason) }))
})
</script>

<template>
  <section class="notice" role="alert" data-testid="inbox-failure-notice">
    <WarningTriangle class="icon icon-sm notice-icon" aria-hidden="true" />
    <div class="notice-main">
      <h3 class="title">{{ common.list.failureTitle }}</h3>
      <ul class="reasons">
        <li v-for="r in rows" :key="r.reason" data-testid="inbox-failure-row">{{ r.text }}</li>
      </ul>
      <BaseButton class="retry" size="sm" :loading="busy" data-testid="inbox-failure-retry" @click="$emit('retry')">{{ common.retry }}</BaseButton>
    </div>
  </section>
</template>

<style scoped>
.notice {
  display: flex;
  align-items: flex-start;
  gap: var(--s-3);
  padding: var(--s-2) var(--s-4);
  background: var(--accent-soft);
  color: var(--ink);
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}

.notice-icon {
  flex: none;
  margin-top: var(--s-1);
  color: var(--mute);
}

.notice-main {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: var(--s-1);
  min-width: 0;
}

/* 本文の下に左寄せ（PC もスマホも同じ）。地（--accent-soft）と枠（--border）が同じ色で枠が消えるので、ボタンの地は --surface にする（ホバーは --accent-soft の地） */
.notice .retry {
  align-self: flex-start;
  margin-top: var(--s-1);
  background: var(--surface);
}

.title {
  margin: 0;
  font-weight: 600;
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}

.reasons {
  margin: 0;
  padding: 0;
  list-style: none;
  overflow-wrap: anywhere;
}
</style>
