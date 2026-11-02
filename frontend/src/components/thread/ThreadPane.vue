<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { thread as msg } from '../../lib/messages'
import { useComposeStore } from '../../stores/compose'
import { useThreadStore } from '../../stores/thread'
import ComposePane from '../compose/ComposePane.vue'
import BaseButton from '../ui/BaseButton.vue'
import Spinner from '../ui/Spinner.vue'
import MessageCard from './MessageCard.vue'
import ReplyBar, { type ReplyMode } from './ReplyBar.vue'
import ThreadHeader from './ThreadHeader.vue'
import { nextExpanded } from './threadView'

const store = useThreadStore()
const compose = useComposeStore()

const detail = computed(() => store.current)
const messages = computed(() => detail.value?.messages ?? [])
const editing = computed(() => store.currentRef !== undefined && compose.isOpenForThread(store.currentRef))

/** 展開中のメッセージ ID。別スレッドでは規則（最新と未読）で作り直し、同一スレッドの再取得では開閉を保つ。 */
const expanded = ref<Set<string>>(new Set())
watch(detail, (cur, prev) => {
  expanded.value = nextExpanded(prev, cur, expanded.value)
}, { immediate: true })

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
    <div v-if="!store.currentRef" class="state empty" data-testid="thread-empty">
      <p class="state-title">{{ msg.empty }}</p>
      <p class="state-hint">{{ msg.emptyHint }}</p>
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
      <ThreadHeader :detail="detail" />
      <div class="scroll" data-testid="thread-scroll">
        <MessageCard
          v-for="m in messages"
          :key="m.id"
          :message="m"
          :expanded="expanded.has(m.id)"
          @link="store.openExternal($event)"
          @save-attachment="store.saveAttachment(m.id, $event)"
        />
      </div>
      <ComposePane v-if="editing" class="embedded-compose" />
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
  background: var(--color-bg-subtle);
}

.state {
  display: flex;
  flex: 1;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: var(--space-3);
  color: var(--color-text-muted);
}

.state-title {
  margin: 0;
  font-size: var(--font-size-lg);
  color: var(--color-text);
}

.state-hint {
  margin: 0;
  font-size: var(--font-size-sm);
}

.scroll {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: var(--space-3);
  min-height: 0;
  overflow-y: auto;
  padding: var(--space-4);
}

.embedded-compose {
  flex: none;
  max-height: 60%;
  border-top: 1px solid var(--color-border);
}
</style>
