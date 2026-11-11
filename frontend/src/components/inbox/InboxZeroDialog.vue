<script setup lang="ts">
import { computed } from 'vue'
import { keyOf } from '../../lib/inboxItems'
import { common, inboxZero as msg } from '../../lib/messages'
import { useInboxZeroStore } from '../../stores/inboxZero'
import BaseButton from '../ui/BaseButton.vue'
import BaseDialog from '../ui/BaseDialog.vue'

// Inbox Zero の 2 つのダイアログ（#314）。
//   consent: 初めて使うとき、Jev へ送る内容への同意（同意前は送らない）。
//   review : 候補の確認の一覧。「差出人・件名」だけをチェックボックスつきで出し（最初は全部チェック済み）、
//            「アーカイブする（Yes）」でチェックの入ったものだけをアーカイブする。「やめる（No）」・閉じる・Esc は何も変えない。
const zero = useInboxZeroStore()
const t = msg

const consentOpen = computed(() => zero.phase === 'consent')
const reviewOpen = computed(() => zero.phase === 'review')
const count = computed(() => zero.checkedCandidates.length)

function onConsentOpen(open: boolean): void {
  if (!open) zero.cancel()
}

function onReviewOpen(open: boolean): void {
  if (!open) zero.cancel()
}
</script>

<template>
  <BaseDialog
    :open="consentOpen"
    size="sm"
    :title="t.consentTitle"
    :description="t.consentDescription"
    @update:open="onConsentOpen"
  >
    <template #footer>
      <BaseButton data-testid="inbox-zero-consent-cancel" @click="zero.cancel()">{{ common.cancel }}</BaseButton>
      <BaseButton variant="primary" data-testid="inbox-zero-consent-agree" @click="zero.agree()">{{ t.consentAgree }}</BaseButton>
    </template>
  </BaseDialog>

  <BaseDialog
    :open="reviewOpen"
    size="md"
    :title="t.dialogTitle"
    :description="t.dialogDescription"
    @update:open="onReviewOpen"
  >
    <p class="count" data-testid="inbox-zero-count" aria-live="polite">
      {{ t.selected(count, zero.candidates.length) }}<template v-if="zero.skipped > 0">　{{ t.skipped(zero.skipped) }}</template>
    </p>
    <ul class="list" :aria-label="t.listLabel" data-testid="inbox-zero-list">
      <li v-for="c in zero.candidates" :key="keyOf(c.ref)">
        <label class="row">
          <input
            type="checkbox"
            class="box"
            :checked="zero.checked.has(keyOf(c.ref))"
            :data-testid="`inbox-zero-item-${keyOf(c.ref)}`"
            @change="zero.toggle(keyOf(c.ref))"
          />
          <span class="text">
            <span class="from">{{ c.from }}</span>
            <span class="subject">{{ c.subject || t.noSubject }}</span>
          </span>
        </label>
      </li>
    </ul>
    <template #footer>
      <BaseButton data-testid="inbox-zero-no" @click="zero.cancel()">{{ t.no }}</BaseButton>
      <BaseButton variant="primary" :disabled="count === 0" data-testid="inbox-zero-yes" @click="zero.confirm()">{{ t.yes }}</BaseButton>
    </template>
  </BaseDialog>
</template>

<style scoped>
.count {
  margin: 0 0 var(--s-2);
  color: var(--mute);
  font-size: var(--font-size-xs);
  line-height: var(--line-height-xs);
}

.list {
  margin: 0;
  padding: 0;
  list-style: none;
  border: 1px solid var(--border);
  border-radius: var(--r-md);
}

.list > li + li {
  border-top: 1px solid var(--border);
}

.row {
  display: flex;
  align-items: flex-start;
  gap: var(--s-3);
  padding: var(--s-2) var(--s-3);
  cursor: pointer;
}

.row:hover {
  background: var(--accent-soft);
}

.box {
  flex: none;
  width: var(--icon-sm);
  height: var(--icon-sm);
  margin: var(--s-1) 0 0;
  accent-color: var(--accent);
  cursor: pointer;
}

.box:focus-visible {
  outline: 2px solid var(--focus);
  outline-offset: 2px;
}

.text {
  display: flex;
  flex-direction: column;
  gap: var(--s-1);
  min-width: 0;
}

.from {
  overflow: hidden;
  font-weight: 600;
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.subject {
  overflow: hidden;
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
