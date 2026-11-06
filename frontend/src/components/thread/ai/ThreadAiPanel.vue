<script setup lang="ts">
import { WarningCircle } from '@iconoir/vue'
import { computed, nextTick, ref, watch } from 'vue'
import { common, compose as composeMsg, thread as msg } from '../../../lib/messages'
import { useSettingsStore } from '../../../stores/settings'
import { useThreadAiStore } from '../../../stores/threadAi'
import { useUiStore } from '../../../stores/ui'
import AIConsentDialog from '../../settings/ai/AIConsentDialog.vue'
import AISetupDialog from '../../settings/ai/AISetupDialog.vue'
import { aiDestination } from '../../settings/ai/aiDestination'
import BaseButton from '../../ui/BaseButton.vue'
import Spinner from '../../ui/Spinner.vue'
import ThreadAiQuestionForm from './ThreadAiQuestionForm.vue'
import type { useThreadAiFlow } from './useThreadAiFlow'

// スレッドの AI パネル（F-17 要約・F-18 質問。#206・#207）。ThreadHeader の下・メッセージ一覧の上。
// 状態は threadAiStore（メモリのみ）。前提確認と同意・初回の案内の画面は flow（ThreadPane が持つ）で動かす。
const props = defineProps<{ flow: ReturnType<typeof useThreadAiFlow> }>()

const store = useThreadAiStore()
const settings = useSettingsStore()
const ui = useUiStore()

const summarizing = computed(() => store.status === 'summarizing')
const asking = computed(() => store.status === 'asking')
const errorText = computed(() => store.error ?? props.flow.error.value)
const needText = computed(() => (props.flow.usingChatGPT.value ? msg.ai.needLogin : msg.ai.needKey))
const truncated = computed(() => {
  const c = store.lastCounts
  return c !== undefined && c.included < c.total ? msg.ai.truncated(c.included, c.total) : undefined
})
const destination = computed(() => {
  const c = settings.aiConfig
  return c ? aiDestination(c.provider, c.baseUrl) : undefined
})

const end = ref<HTMLElement>()
// 新しい回答・要約が加わったら、パネルの末尾が見える位置までスクロールする（すでに見えていれば動かさない）。
watch(
  () => [store.turns.length, store.summary],
  () => void nextTick(() => end.value?.scrollIntoView?.({ block: 'nearest' })),
)

function ask(): void {
  void props.flow.start('ask')
}
</script>

<template>
  <section class="ai-panel" :aria-label="msg.ai.panelLabel" data-testid="thread-ai-panel">
    <p v-if="truncated" class="notice" data-testid="thread-ai-truncated">{{ truncated }}</p>

    <div v-if="store.summary || summarizing" class="block" data-testid="thread-ai-summary">
      <div class="block-head">
        <h3 class="heading">{{ msg.ai.summaryHeading }}</h3>
        <template v-if="summarizing">
          <span class="progress" role="status"><Spinner /> {{ msg.ai.summarizing }}</span>
          <BaseButton size="sm" variant="secondary" data-testid="thread-ai-cancel" @click="store.cancel()">{{ msg.ai.cancel }}</BaseButton>
        </template>
        <BaseButton
          v-else
          size="sm"
          variant="ghost"
          :disabled="store.busy"
          data-testid="thread-ai-resummarize"
          @click="flow.start('summarize')"
        >
          {{ msg.ai.resummarize }}
        </BaseButton>
      </div>
      <p v-if="store.summary" class="text" data-testid="thread-ai-summary-text">{{ store.summary.text }}</p>
    </div>

    <div v-if="store.turns.length > 0 || asking" class="block conversation" data-testid="thread-ai-conversation">
      <div class="block-head">
        <p v-if="store.droppedFromContext" class="notice" data-testid="thread-ai-dropped">{{ msg.ai.droppedFromContext }}</p>
        <BaseButton
          v-if="store.turns.length > 0"
          size="sm"
          variant="ghost"
          :disabled="asking"
          data-testid="thread-ai-new-conversation"
          @click="store.newConversation()"
        >
          {{ msg.ai.newConversation }}
        </BaseButton>
      </div>
      <div v-for="(t, i) in store.turns" :key="i" class="turn" :data-role="t.role">
        <h4 class="mute-heading">{{ t.role === 'user' ? msg.ai.questionHeading : msg.ai.answerHeading }}</h4>
        <p class="text">{{ t.text }}</p>
      </div>
      <div v-if="asking" class="block-head">
        <span class="progress" role="status"><Spinner /> {{ msg.ai.asking }}</span>
        <BaseButton size="sm" variant="secondary" data-testid="thread-ai-cancel" @click="store.cancel()">{{ msg.ai.cancel }}</BaseButton>
      </div>
    </div>

    <!-- インラインアラートの出現はフェードイン・消去はフェードアウト（ガイド M5） -->
    <Transition name="alert">
      <div v-if="errorText" class="error" role="alert" data-testid="thread-ai-error">
        <WarningCircle class="icon error-icon" aria-hidden="true" />
        <div class="error-main">
          <strong class="error-title">{{ common.alertTitle }}</strong>
          <p class="error-body">{{ msg.ai.failed }}（{{ errorText }}）</p>
        </div>
      </div>
    </Transition>

    <Transition name="alert">
      <p v-if="flow.needsKey.value" class="notice notice-need-key" data-testid="thread-ai-need-key">
        {{ needText }}
        <BaseButton size="sm" variant="ghost" @click="ui.openSettings()">{{ msg.ai.openSettings }}</BaseButton>
      </p>
    </Transition>

    <ThreadAiQuestionForm v-model="store.draft" :disabled="store.busy || flow.needsKey.value" @submit="ask" />
    <div ref="end" />

    <AISetupDialog :open="flow.step.value === 'setup'" @choose="flow.answerSetup($event)" />
    <AIConsentDialog
      v-if="destination"
      :open="flow.step.value === 'consent'"
      :provider-label="destination.providerLabel"
      :url="destination.url"
      :agree-label="composeMsg.ai.consentAgree"
      @agree="flow.answerConsent(true)"
      @cancel="flow.answerConsent(false)"
    />
  </section>
</template>

<style scoped>
.ai-panel {
  display: flex;
  flex: none;
  flex-direction: column;
  gap: var(--gap-card-section);
  padding: var(--pad-card-sm);
  border: 1px solid var(--border);
  border-radius: var(--r-lg);
  background: var(--surface);
}

.block {
  display: flex;
  flex-direction: column;
  gap: var(--s-2);
}

/* 質問/回答の組どうしは段落間（16）。組の中の見出し→本文（8）と区別する。 */
.conversation {
  gap: var(--gap-para);
}

.block-head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--s-2);
}

.heading {
  margin: 0;
  margin-right: auto;
  font-size: var(--font-size-sm);
  color: var(--ink);
}

.mute-heading {
  margin: 0;
  font-size: var(--font-size-xs);
  line-height: var(--line-height-xs);
  font-weight: 600;
  color: var(--mute);
}

.turn {
  display: flex;
  flex-direction: column;
  gap: var(--s-2);
}

.text {
  margin: 0;
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
  color: var(--ink);
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.progress {
  display: inline-flex;
  align-items: center;
  gap: var(--s-2);
  color: var(--mute);
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}

.notice {
  margin: 0;
  color: var(--mute);
  font-size: var(--font-size-xs);
  line-height: var(--line-height-xs);
}

/* 2 文の案内（次の操作を伝える文）は --ink・sm（C6）。 */
.notice-need-key {
  color: var(--ink);
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}

.error {
  display: flex;
  align-items: flex-start;
  gap: var(--s-3);
  padding: var(--s-3) var(--s-4);
  border: 1px solid var(--danger-border);
  border-radius: var(--r-md);
  background: var(--danger-soft);
  color: var(--ink);
}

.error-icon {
  flex: none;
  margin-top: var(--s-1);
  color: var(--danger);
}

.error-main {
  flex: 1;
  min-width: 0;
}

.error-title {
  display: block;
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
  font-weight: 600;
}

.error-body {
  margin: 0;
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
  overflow-wrap: anywhere;
}

.alert-enter-active {
  animation: ui-fade-in var(--motion-fade-in) var(--ease-out);
}

.alert-leave-active {
  animation: ui-fade-out var(--motion-fade-out) var(--ease-in) forwards;
}
</style>
