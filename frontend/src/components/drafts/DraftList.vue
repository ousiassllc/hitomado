<script setup lang="ts">
import { EditPencil, Refresh } from '@iconoir/vue'
import { ref } from 'vue'
import type { DraftSummary } from '../../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import { common, drafts as msg } from '../../lib/messages'
import { vOverlayScroll } from '../../lib/overlayScroll'
import { useAccountsStore } from '../../stores/accounts'
import { DRAFT_LIST_LIMIT, useDraftsStore } from '../../stores/drafts'
import BaseIconButton from '../ui/BaseIconButton.vue'
import ConfirmDialog from '../ui/ConfirmDialog.vue'
import EmptyState from '../ui/EmptyState.vue'
import Spinner from '../ui/Spinner.vue'
import DraftRow from './DraftRow.vue'

// 下書きの一覧（F-12。#366）。全アカウントの Gmail の下書きを、更新の新しい順に 1 つの一覧で出す。行を開くと作成画面（返信の下書きはそのスレッドの返信）に復元する。
// 破棄は Gmail の下書きも消すので、確認のモーダルを挟む（ガイド O2。作成画面の「破棄」と同じ）。
const store = useDraftsStore()
const accounts = useAccountsStore()

function accountOf(d: DraftSummary) {
  return accounts.byId(d.accountId)
}

/** 破棄の確認待ちの下書き（確認のモーダルを開いている間だけ持つ）。 */
const pending = ref<DraftSummary>()
const discarding = ref(false)

async function confirmDiscard(): Promise<void> {
  const d = pending.value
  if (!d || discarding.value) return
  discarding.value = true
  try {
    await store.discard(d)
  } finally {
    discarding.value = false
    pending.value = undefined
  }
}
</script>

<template>
  <div class="drafts" data-testid="draft-list">
    <header class="bar">
      <h2 class="title">{{ msg.title }}</h2>
      <BaseIconButton :label="store.loading ? msg.refreshing : msg.refresh" :disabled="store.loading" data-testid="drafts-refresh" @click="store.load()">
        <Refresh class="icon" aria-hidden="true" />
      </BaseIconButton>
    </header>
    <div v-overlay-scroll="{ fade: 'var(--surface)' }" class="scroll">
      <div v-if="store.loading && store.items.length === 0" class="state" role="status" :aria-label="common.loading"><Spinner /></div>
      <div v-else-if="store.items.length === 0" class="state">
        <EmptyState>
          <template #icon><EditPencil class="icon icon-lg" /></template>
          {{ msg.empty }}
        </EmptyState>
      </div>
      <ul v-else class="list" :aria-label="msg.label" :aria-busy="store.loading">
        <DraftRow
          v-for="d in store.items"
          :key="`${d.accountId}/${d.draftId}`"
          :draft="d"
          :account="accountOf(d)"
          :selected="store.isOpen(d)"
          @open="store.open(d)"
          @discard="pending = d"
        />
      </ul>
      <p v-if="store.truncated" class="note">{{ msg.truncated(DRAFT_LIST_LIMIT) }}</p>
    </div>
    <ConfirmDialog
      :open="pending !== undefined"
      :title="msg.discardTitle"
      :detail="msg.discardDetail"
      :confirm-label="msg.discardConfirm"
      :busy="discarding"
      @update:open="(v) => !v && !discarding && (pending = undefined)"
      @confirm="confirmDiscard"
    />
  </div>
</template>

<style scoped>
.drafts {
  display: flex;
  flex: 1;
  flex-direction: column;
  min-height: 0;
}

.bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--s-2);
  padding: var(--s-1) var(--s-2) var(--s-1) var(--pad-card-sm);
  border-bottom: 1px solid var(--border);
}

.title {
  margin: 0;
  font-size: var(--font-size-base);
  font-weight: 600;
}

.scroll {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
}

.list {
  margin: 0;
  padding: 0;
  list-style: none;
}

.state {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: var(--s-6);
  color: var(--mute);
}

.note {
  margin: 0;
  padding: var(--s-3) var(--pad-card-sm);
  color: var(--mute);
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}
</style>
