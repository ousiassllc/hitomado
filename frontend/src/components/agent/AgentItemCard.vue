<script setup lang="ts">
import { computed, ref, useId, watch } from 'vue'
import type { AgentItem } from '../../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import { list } from '../../lib/api'
import { agent } from '../../lib/messages'
import { useAccountsStore } from '../../stores/accounts'
import { useAgentStore } from '../../stores/agent'
import { useComposeStore } from '../../stores/compose'
import { useInboxStore } from '../../stores/inbox'
import { useUiStore } from '../../stores/ui'
import AccountChip from '../account/AccountChip.vue'
import BaseButton from '../ui/BaseButton.vue'

// エージェントのパネルの 1 行（新着 1 件）。状態ごとに出すものを変える: 質問（答えの入力）・確認待ち（下書きの抜粋）・
// あとで・返信不要（「やっぱり返信する」）・失敗（「やり直す」）。送信の操作は無い（下書きを開いて、送るのは利用者）。
const props = defineProps<{ item: AgentItem }>()

const msg = agent.panel
const store = useAgentStore()
const accounts = useAccountsStore()
const compose = useComposeStore()
const inbox = useInboxStore()
const ui = useUiStore()
const idBase = useId()

const questions = computed(() => list(props.item.questions))
const answers = ref<string[]>([])
watch(questions, (q) => (answers.value = q.map(() => '')), { immediate: true })
const answerable = computed(
  () => questions.value.length > 0 && answers.value.every((a) => a.trim() !== ''),
)

const hasDraft = computed(() => !!props.item.draftId)
/** 質問を出す: 質問待ち、または質問の途中で「あとで」にして下書きがまだ無いもの。 */
const showQuestions = computed(
  () =>
    questions.value.length > 0 &&
    !hasDraft.value &&
    (props.item.status === 'asking' || props.item.status === 'later'),
)
const showDraft = computed(
  () => hasDraft.value && (props.item.status === 'ready' || props.item.status === 'later'),
)
/** 「開いて確認」を出す: 確認待ち、または下書きのある「あとで」。確認待ちで下書きが見つからないときは、押したときに知らせる。 */
const canOpen = computed(() => props.item.status === 'ready' || showDraft.value)
const canDefer = computed(() => props.item.status === 'asking' || props.item.status === 'ready')
/** 判断を直せる: 返信不要・失敗、または質問も下書きも無い「あとで」。 */
const canReconsider = computed(
  () =>
    props.item.status === 'skipped' ||
    props.item.status === 'failed' ||
    (props.item.status === 'later' && !showQuestions.value && !canOpen.value),
)

const opening = ref(false)
const busy = computed(() => store.isBusy(props.item.id) || opening.value)

const subject = computed(() => props.item.subject || msg.noSubject)
const recipients = computed(() =>
  list(props.item.to)
    .map((a) => a.name || a.email)
    .join('、'),
)
const chipAccount = computed(() => ({
  displayName: props.item.accountName || props.item.accountEmail,
  email: props.item.accountEmail,
  color: accounts.colorOf(props.item.accountId),
}))

function submitAnswers(): void {
  if (!answerable.value || busy.value) return
  void store.answer(
    props.item.id,
    answers.value.map((a) => a.trim()),
  )
}

/**
 * 「開いて確認」。返信の作成画面を、エージェントが作った下書きつきで開く。開けたらパネルを閉じて（作成画面を隠さない）確認済みにする。
 * 開けなかった（下書きが無い・取得に失敗）ときは、作成画面側のエラーか、下書きが無い旨のアラートで知らせる。
 */
async function openDraft(): Promise<void> {
  if (busy.value) return
  const { accountId, threadId, draftId } = props.item
  if (!draftId) {
    ui.pushAlert(msg.noDraft)
    return
  }
  opening.value = true
  try {
    const ref = { accountId, threadId }
    if (!(await compose.open({ mode: 'reply_all', ref, draftId }))) return
    inbox.select(ref)
    ui.closeAgent()
    await store.confirm(props.item.id)
  } finally {
    opening.value = false
  }
}
</script>

<template>
  <li class="card" :data-status="item.status" :data-testid="`agent-item-${item.id}`">
    <h4 class="subject">{{ subject }}</h4>
    <p class="meta">{{ msg.from(item.from) }}</p>
    <p v-if="showDraft && recipients" class="meta">{{ msg.to(recipients) }}</p>
    <p class="meta account">
      {{ msg.receivedBy }}
      <AccountChip :account="chipAccount" size="sm" />
    </p>
    <p v-if="item.reason" class="reason" data-testid="agent-reason">{{ item.reason }}</p>

    <p v-if="showDraft && item.draftPreview" class="preview" data-testid="agent-preview">
      {{ item.draftPreview }}
    </p>

    <form v-if="showQuestions" class="questions" @submit.prevent="submitAnswers">
      <p class="hint">{{ msg.questionsHint }}</p>
      <div v-for="(q, i) in questions" :key="i" class="question">
        <label class="question-label" :for="`${idBase}-${i}`">{{ q }}</label>
        <textarea
          :id="`${idBase}-${i}`"
          v-model="answers[i]"
          class="question-input"
          rows="2"
          :placeholder="msg.answerPlaceholder"
          :disabled="busy"
          data-testid="agent-answer"
        />
      </div>
      <div class="actions">
        <BaseButton type="submit" variant="primary" size="sm" :loading="busy" :disabled="!answerable" data-testid="agent-answer-submit">
          {{ msg.answerSubmit }}
        </BaseButton>
        <BaseButton v-if="canDefer" size="sm" :disabled="busy" data-testid="agent-defer" @click="store.defer(item.id)">
          {{ msg.defer }}
        </BaseButton>
      </div>
    </form>

    <div v-else class="actions">
      <BaseButton v-if="canOpen" variant="primary" size="sm" :loading="busy" data-testid="agent-open-draft" @click="openDraft">
        {{ msg.openDraft }}
      </BaseButton>
      <BaseButton v-if="canReconsider" variant="primary" size="sm" :loading="busy" data-testid="agent-reconsider" @click="store.reconsider(item.id)">
        {{ item.status === 'failed' ? msg.retry : msg.reconsider }}
      </BaseButton>
      <BaseButton v-if="canDefer" size="sm" :disabled="busy" data-testid="agent-defer" @click="store.defer(item.id)">
        {{ msg.defer }}
      </BaseButton>
    </div>
  </li>
</template>

<style scoped>
.card {
  display: flex;
  flex-direction: column;
  gap: var(--s-2);
  padding: var(--pad-card-sm);
  border: 1px solid var(--border);
  border-radius: var(--r-lg);
  background: var(--surface);
  list-style: none;
}

.subject {
  margin: 0;
  font-size: var(--font-size-base);
  line-height: var(--line-height-base);
  font-weight: 600;
  overflow-wrap: anywhere;
}

.meta,
.hint {
  margin: 0;
  color: var(--mute);
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
  overflow-wrap: anywhere;
}

.account {
  display: flex;
  align-items: center;
  gap: var(--s-2);
  min-width: 0;
}

.reason {
  margin: 0;
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}

/* 下書きの最初の数行（プレーンテキスト）。 */
.preview {
  display: -webkit-box;
  margin: 0;
  padding: var(--s-2) var(--s-3);
  border: 1px solid var(--border);
  border-radius: var(--r-md);
  background: var(--bone);
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
  white-space: pre-wrap;
  overflow: hidden;
  overflow-wrap: anywhere;
  -webkit-line-clamp: 4;
  -webkit-box-orient: vertical;
}

.questions {
  display: flex;
  flex-direction: column;
  gap: var(--gap-field);
}

.question {
  display: flex;
  flex-direction: column;
  gap: var(--gap-label);
}

.question-label {
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
  overflow-wrap: anywhere;
}

.question-input {
  box-sizing: border-box;
  width: 100%;
  padding: var(--s-3);
  border: 1px solid var(--border);
  border-radius: var(--r-md);
  background: var(--surface);
  color: var(--ink);
  font: inherit;
  font-size: var(--font-size-base);
  line-height: var(--line-height-base);
  resize: vertical;
}

.question-input:focus {
  border-color: var(--focus);
  outline: 2px solid var(--focus);
  outline-offset: -1px;
}

.question-input:disabled {
  opacity: 0.5;
}

.actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--s-2);
}
</style>
