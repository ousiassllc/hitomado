<script setup lang="ts">
import { MailOpen } from '@iconoir/vue'
import { computed, nextTick, ref, watch } from 'vue'
import { thread as msg } from '../../lib/messages'
import { vOverlayScroll } from '../../lib/overlayScroll'
import { useComposeStore } from '../../stores/compose'
import { useLayoutStore } from '../../stores/layout'
import { useThreadStore } from '../../stores/thread'
import ComposePane from '../compose/ComposePane.vue'
import BaseButton from '../ui/BaseButton.vue'
import EmptyState from '../ui/EmptyState.vue'
import Spinner from '../ui/Spinner.vue'
import ThreadAiPanel from './ai/ThreadAiPanel.vue'
import { useThreadAiFlow } from './ai/useThreadAiFlow'
import MessageCard from './MessageCard.vue'
import ReplyBar, { type ReplyMode } from './ReplyBar.vue'
import ThreadHeader from './ThreadHeader.vue'
import { nextExpanded } from './threadView'

const store = useThreadStore()
const compose = useComposeStore()
const layout = useLayoutStore()
const ai = useThreadAiFlow()

const detail = computed(() => store.current)
const messages = computed(() => detail.value?.messages ?? [])
/** 開いているスレッドの識別（別のメールを開いたら、はみ出していればスクロールバーを 1 秒出す。同じスレッドの再取得では出さない）。 */
const openKey = computed(() => (detail.value ? `${detail.value.account.id}/${detail.value.threadId}` : ''))
/** 返信・転送の編集を、スレッドの下に埋め込んで出す。1 画面ずつのレイアウトでは AppShell が全画面で出す。 */
const editing = computed(
  () => !layout.isStack && store.currentRef !== undefined && compose.isOpenForThread(store.currentRef),
)

/** 展開中のメッセージ ID。別スレッドでは規則（最新と未読）で作り直し、同一スレッドの再取得では開閉を保つ。 */
const expanded = ref<Set<string>>(new Set())
watch(detail, (cur, prev) => {
  expanded.value = nextExpanded(prev, cur, expanded.value)
}, { immediate: true })

/**
 * 作成画面を右ペイン全体へ広げているか（#112）。コンポーネント内のメモリだけで持ち、保存しない。
 * スレッドの切り替え・送信・閉じる・破棄（= 埋め込みの作成画面が無くなる）で元に戻す。
 */
const composeExpanded = ref(false)
const scrollEl = ref<HTMLElement>()
let savedScroll = 0

async function setComposeExpanded(on: boolean): Promise<void> {
  if (composeExpanded.value === on) return
  if (on) savedScroll = scrollEl.value?.scrollTop ?? 0
  composeExpanded.value = on
  if (on) return
  // 隠していたスレッド本文を、広げる前と同じ読んでいた位置で出す。
  await nextTick()
  if (scrollEl.value) scrollEl.value.scrollTop = savedScroll
}
watch(editing, (v) => {
  if (!v) void setComposeExpanded(false)
})
watch(
  () => store.currentRef,
  (cur, prev) => {
    if (cur?.accountId !== prev?.accountId || cur?.threadId !== prev?.threadId) void setComposeExpanded(false)
  },
)

function openReply(mode: ReplyMode): void {
  const ref = store.currentRef
  if (ref) void compose.open({ mode, ref })
}

function openDraft(): void {
  const ref = store.currentRef
  const draftId = detail.value?.draftId
  if (ref && draftId) void compose.open({ mode: 'reply_all', ref, draftId })
}
</script>

<template>
  <section class="thread-pane" :aria-label="detail?.subject ?? msg.empty">
    <div v-if="!store.currentRef" class="state" data-testid="thread-empty">
      <EmptyState>
        <template #icon><MailOpen class="icon icon-lg" /></template>
        <p class="state-title">{{ msg.empty }}</p>
        <p>{{ msg.emptyHint }}</p>
      </EmptyState>
    </div>

    <div v-else-if="!detail && store.loading" class="state" data-testid="thread-loading">
      <Spinner />
      <span>{{ msg.loading }}</span>
    </div>

    <div v-else-if="!detail" class="state" role="alert" data-testid="thread-error">
      <p class="state-title">{{ store.error?.message || msg.loadFailed }}</p>
      <BaseButton @click="store.reload()">{{ msg.retry }}</BaseButton>
    </div>

    <template v-else>
      <ThreadHeader
        v-show="!composeExpanded"
        :detail="detail"
        :summarize-disabled="ai.summarizeDisabled.value"
        :summarize-hint="ai.summarizeHint.value"
        @summarize="ai.start('summarize')"
      />
      <div v-show="!composeExpanded" ref="scrollEl" v-overlay-scroll="{ flashOn: openKey }" class="scroll" data-testid="thread-scroll">
        <ThreadAiPanel :flow="ai" />
        <MessageCard
          v-for="m in messages"
          :key="m.id"
          :message="m"
          :expanded="expanded.has(m.id)"
          @link="store.openExternal($event)"
          @save-attachment="store.saveAttachment(m.id, $event)"
        />
      </div>
      <ComposePane
        v-if="editing"
        embedded
        :expanded="composeExpanded"
        @toggle-expand="setComposeExpanded(!composeExpanded)"
      />
      <ReplyBar v-else :draft-id="detail.draftId" @open="openReply" @open-draft="openDraft" />
    </template>
  </section>
</template>

<style scoped>
.thread-pane {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
  background: var(--bone);
}

.state {
  display: flex;
  flex: 1;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: var(--s-3);
  color: var(--mute);
}

.state-title {
  margin: 0;
  font-size: var(--font-size-base);
  color: var(--ink);
}

.scroll {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: var(--s-3);
  min-height: 0;
  overflow-y: auto;
  padding: var(--s-4);
}
</style>
