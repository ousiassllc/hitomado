<script setup lang="ts">
import { Trash } from '@iconoir/vue'
import { computed } from 'vue'
import type { DraftSummary } from '../../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import { list } from '../../lib/api'
import { formatDate } from '../../lib/format'
import { drafts as msg } from '../../lib/messages'
import AccountChip from '../account/AccountChip.vue'
import BaseIconButton from '../ui/BaseIconButton.vue'

// 下書き一覧の 1 行（F-12。#366）。行の本体は「開く」ボタン（宛先・件名・本文の先頭）で、右端に「破棄」を置く（確認は DraftList）。
const props = defineProps<{
  draft: DraftSummary
  account?: { displayName: string; email: string; color: string }
  /** 作成画面で開いている下書き */
  selected: boolean
}>()
const emit = defineEmits<{ open: []; discard: [] }>()

const recipients = computed(
  () =>
    list(props.draft.to)
      .map((a) => a.name || a.email)
      .filter((s) => s !== '')
      .join('、') || msg.noRecipient,
)
const subject = computed(() => props.draft.subject || msg.noSubject)
const label = computed(() => msg.open(props.account?.displayName ?? '', recipients.value, subject.value))
</script>

<template>
  <li class="row" :class="{ selected }" :data-testid="`draft-row-${draft.draftId}`">
    <button type="button" class="open" :aria-label="label" :aria-current="selected || undefined" @click="emit('open')">
      <span class="line1">
        <AccountChip v-if="account" :account="account" size="sm" />
        <span class="to">{{ recipients }}</span>
        <time class="date">{{ formatDate(draft.updatedAt) }}</time>
      </span>
      <span class="line2">
        <span class="subject">{{ subject }}</span>
        <span class="snippet">{{ draft.snippet || msg.noBody }}</span>
      </span>
    </button>
    <BaseIconButton class="discard" :label="msg.discard" @click="emit('discard')">
      <Trash class="icon" aria-hidden="true" />
    </BaseIconButton>
  </li>
</template>

<style scoped>
.row {
  display: flex;
  align-items: center;
  gap: var(--s-1);
  padding-right: var(--s-2);
  border-bottom: 1px solid var(--border);
  background: var(--surface);
  /* 補助文字（日時・本文の先頭）の色。--accent-soft 地の上では --mute が AA に届かないので、ホバー・選択のときだけ --mute-on-soft にする（ThreadRow と同じ）。 */
  --row-mute: var(--mute);
}

.row:hover,
.row:focus-within,
.row.selected {
  --row-mute: var(--mute-on-soft);
  background: var(--accent-soft);
}

.open {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: var(--s-1);
  min-width: 0;
  padding: var(--s-3) 0 var(--s-3) var(--pad-card-sm);
  border: none;
  background: transparent;
  color: var(--ink);
  font: inherit;
  text-align: left;
  cursor: pointer;
}

.line1,
.line2 {
  display: flex;
  align-items: center;
  gap: var(--gap-inline);
  min-width: 0;
}

.to {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: var(--font-size-sm);
}

.date {
  flex: none;
  color: var(--row-mute);
  font-family: var(--font-mono), var(--font-sans);
  font-size: var(--font-size-xs);
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
  color: var(--row-mute);
  font-size: var(--font-size-sm);
}
</style>
