<script setup lang="ts">
import { Attachment, WarningTriangle } from '@iconoir/vue'
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
  /** 一覧の中の位置（1 始まり）。仮想スクロールで一部の行だけ描くので、読み上げ側へ aria-posinset で伝える */
  posinset?: number
  /** 一覧の件数。続きがあって総数が分からないときは -1（aria-setsize の規定） */
  setsize?: number
}>()

const sender = computed(() => props.thread.lastFrom.name || props.thread.lastFrom.email || common.list.noSender)
const needsReauth = computed(() => props.account?.status === 'needs_reauth')
const labels = computed(() => props.thread.labels ?? [])
const label = computed(
  () =>
    `${props.thread.isUnread ? `${common.list.unreadMark}、` : ''}${props.account?.displayName ?? ''} ${sender.value}、${
      props.thread.subject || common.list.noSubject
    }${needsReauth.value ? `、${common.list.needsReauthRow}` : ''}`,
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
      'needs-reauth': needsReauth,
    }"
    role="option"
    tabindex="-1"
    :aria-selected="selected || checked || false"
    :aria-posinset="posinset"
    :aria-setsize="setsize"
    :aria-label="label"
  >
    <span class="band" :style="{ background: account ? accountColorVar(account.color) : 'var(--border)' }" aria-hidden="true" />
    <div class="main">
      <div class="line1">
        <AccountChip v-if="account" :account="account" size="sm" />
        <span class="sender">{{ sender }}</span>
        <span v-if="thread.messageCount > 1" class="count" :title="common.list.messageCount(thread.messageCount)">{{
          thread.messageCount
        }}</span>
        <span v-if="thread.hasAttachment" class="clip" role="img" :aria-label="common.list.attachment"><Attachment class="icon icon-sm" aria-hidden="true" /></span>
        <time class="date">{{ formatDate(thread.lastMessageAt) }}</time>
      </div>
      <div class="line2">
        <span v-if="thread.isUnread" class="dot" aria-hidden="true" />
        <span v-if="needsReauth" class="reauth"><WarningTriangle class="icon icon-sm" aria-hidden="true" />{{ common.list.needsReauthRow }}</span>
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
  border-bottom: 1px solid var(--border);
  background: var(--surface);
  cursor: pointer;
  overflow: hidden;
  /* 補助文字（日時・スニペット・件数・既読の件名）の色。--accent-soft 地の上では --mute が AA に届かないので、ホバー・選択・複数選択のときだけ --mute-on-soft にする（#267）。 */
  --row-mute: var(--mute);
}

.row:hover,
.row.selected,
.row.checked {
  --row-mute: var(--mute-on-soft);
  background: var(--accent-soft);
}

.row.active {
  outline: 2px solid var(--focus);
  outline-offset: -2px;
}

.band {
  flex: none;
  width: 6px;
}

.main {
  flex: 1;
  min-width: 0;
  padding: var(--s-1) var(--s-3);
  display: flex;
  flex-direction: column;
  justify-content: center;
  gap: var(--s-1);
}

.line1,
.line2,
.labels {
  display: flex;
  align-items: center;
  gap: var(--s-2);
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
  color: var(--row-mute);
  font-family: var(--font-mono);
  font-size: var(--font-size-xs);
}

.clip {
  flex: none;
  font-size: var(--font-size-xs);
}

.date {
  flex: none;
  color: var(--row-mute);
  font-family: var(--font-mono), var(--font-sans);
  font-size: var(--font-size-xs);
}

.dot {
  flex: none;
  width: 8px;
  height: 8px;
  border-radius: var(--r-full);
  background: var(--accent);
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

/* 未読は太字にせず、未読ドットと文字色の対比（未読 = --ink、既読 = --row-mute）で区別する。 */
.sender {
  color: var(--row-mute);
}

.subject {
  color: var(--row-mute);
}

.unread .sender,
.unread .subject {
  color: var(--ink);
}

/* 要再接続のアカウントの行は、アイコンと文言で伝える（色や濃淡だけにしない）。文字は --mute にして、操作できる行と区別する。差出人と件名は --row-mute なので、通常時は --mute のまま、--accent-soft 地（ホバー・選択・複数選択）では --mute-on-soft で AA に届く（#316）。 */
.reauth {
  display: inline-flex;
  flex: none;
  align-items: center;
  gap: var(--s-1);
  color: var(--ink);
  font-size: var(--font-size-xs);
}

.reauth .icon {
  color: var(--warning);
}

.row.needs-reauth .sender {
  color: var(--row-mute);
}

.row.needs-reauth .subject {
  color: var(--row-mute);
}
</style>
