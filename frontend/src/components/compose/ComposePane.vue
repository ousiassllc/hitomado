<script setup lang="ts">
import { computed, ref } from 'vue'
import { common, compose as msg } from '../../lib/messages'
import { SEND_SHORTCUT_KEY } from '../../lib/shortcuts'
import { useComposeStore } from '../../stores/compose'
import BaseButton from '../ui/BaseButton.vue'
import BaseTooltip from '../ui/BaseTooltip.vue'
import AttachmentBar from './AttachmentBar.vue'
import AiDraftButton from './ai/AiDraftButton.vue'
import ComposeFields from './ComposeFields.vue'
import RichTextEditor from './RichTextEditor.vue'
import SignatureConflictDialog from './SignatureConflictDialog.vue'

const props = defineProps<{
  /** スレッドの下に埋め込んで出すか（返信・転送。ThreadPane）。埋め込みのときだけ「広げる／元に戻す」を出す。 */
  embedded?: boolean
  /** 埋め込みのとき、右ペイン全体へ広げているか（状態は呼び出し側が持つ）。 */
  expanded?: boolean
}>()
const emit = defineEmits<{ 'toggle-expand': [] }>()

const compose = useComposeStore()

const session = computed(() => compose.session)
const title = computed(() => {
  switch (session.value?.mode) {
    case 'reply':
      return msg.titleReply
    case 'reply_all':
      return msg.titleReplyAll
    case 'forward':
      return msg.titleForward
    default:
      return msg.titleNew
  }
})
const canAi = computed(() => session.value?.mode === 'reply' || session.value?.mode === 'reply_all')
/** 送信確認の表示中・送信中はエディタを操作できない。 */
const frozen = computed(() => compose.sending || compose.preview !== undefined)
/** 本文エディタ: 送信確認中・送信中に加え、AI 生成中も読み取り専用（生成のキャンセルは可能）。 */
const bodyFrozen = computed(() => frozen.value || compose.aiGenerating)

function onKeydown(e: KeyboardEvent): void {
  // Ctrl+Enter は送信確認を開く（確認なしの送信はできない）
  if (e.key === 'Enter' && (e.ctrlKey || e.metaKey) && !e.isComposing) {
    e.preventDefault()
    send(false)
  }
}

const fields = ref<InstanceType<typeof ComposeFields>>()

/** 入力途中の宛先を確定してから送信確認を開く（確定後の content を使う）。 */
function send(archive: boolean): void {
  if (!compose.canSend || frozen.value) return
  fields.value?.commitPending()
  void compose.prepareSend(archive)
}
</script>

<template>
  <section
    v-if="session"
    class="compose-pane"
    :class="{ embedded: props.embedded, expanded: props.embedded && props.expanded }"
    data-file-drop-target
    :aria-label="msg.paneLabel"
    @keydown="onKeydown"
  >
    <header class="compose-head">
      <h2 class="compose-title">{{ title }}</h2>
      <span v-if="compose.saving" class="compose-status" role="status">{{ msg.saving }}</span>
      <span v-else-if="compose.dirty" class="compose-dirty" aria-hidden="true" />
      <span class="spacer" />
      <BaseButton
        v-if="props.embedded"
        size="sm"
        variant="ghost"
        :aria-pressed="props.expanded === true"
        data-testid="compose-expand"
        @click="emit('toggle-expand')"
      >
        {{ props.expanded ? msg.collapse : msg.expand }}
      </BaseButton>
      <BaseButton size="sm" variant="ghost" :disabled="compose.sending" @click="compose.close()">{{ msg.close }}</BaseButton>
      <BaseButton size="sm" variant="ghost" :disabled="compose.sending" @click="compose.discard()">{{ msg.discard }}</BaseButton>
    </header>

    <ComposeFields ref="fields" :frozen="frozen" />

    <AiDraftButton
      v-if="canAi"
      :key="session.sessionId"
      :session-id="session.sessionId"
      @generated="compose.insertGenerated"
    />

    <RichTextEditor
      class="compose-editor"
      :html="compose.content.bodyHtml"
      :editable="!bodyFrozen"
      @update:html="compose.content.bodyHtml = $event"
    />

    <AttachmentBar />

    <p v-if="compose.error" class="compose-error" role="alert">{{ compose.error }}</p>
    <p v-if="compose.saveError" class="compose-error" role="alert">
      {{ msg.saveFailed }}（{{ compose.saveError }}）
    </p>

    <footer class="compose-foot">
      <span v-if="session.accountId === undefined" class="foot-hint" role="note">{{ msg.senderRequiredHint }}</span>
      <span class="spacer" />
      <BaseButton
        v-if="compose.canArchive"
        variant="secondary"
        :disabled="!compose.canSend || frozen"
        :loading="compose.preparing || compose.sending"
        @click="send(true)"
      >
        {{ msg.sendAndArchive }}
      </BaseButton>
      <BaseTooltip :text="common.shortcutHint(msg.send, SEND_SHORTCUT_KEY)">
        <BaseButton
          variant="primary"
          :disabled="!compose.canSend || frozen"
          :loading="compose.preparing || compose.sending"
          @click="send(false)"
        >
          {{ compose.sending ? msg.sending : msg.send }}
        </BaseButton>
      </BaseTooltip>
    </footer>

    <SignatureConflictDialog />
  </section>
</template>

<style scoped>
/* 高さが足りないときは、本文エディタを縮めず、作成画面全体が縦にスクロールする（F-11。#112）。 */
.compose-pane {
  display: flex;
  flex: 1 1 auto;
  flex-direction: column;
  gap: var(--s-3);
  min-height: 0;
  padding: var(--s-4);
  overflow-y: auto;
  background: var(--surface);
  color: var(--ink);
}

.compose-pane > * {
  flex-shrink: 0;
}

/* スレッド内の作成画面: 右ペインの 60% までにする。ただし本文エディタの最低高さが優先で、収まらない分はスクロールする。 */
.compose-pane.embedded {
  flex: none;
  max-height: 60%;
  border-top: 1px solid var(--border);
}

/* 広げる: 右ペイン全体を使う（スレッドの本文は ThreadPane が隠す）。 */
.compose-pane.embedded.expanded {
  flex: 1 1 auto;
  max-height: none;
}

/* ドロップ領域: ドラッグ中は --accent-soft の地に --accent の破線（1px）。 */
.compose-pane.file-drop-target-active {
  background: var(--accent-soft);
  outline: 1px dashed var(--accent);
  outline-offset: -4px;
}

.compose-head,
.compose-foot {
  display: flex;
  align-items: center;
  gap: var(--s-2);
}

.compose-title {
  margin: 0;
  font-size: var(--font-size-base);
}

.compose-status,
.foot-hint {
  color: var(--mute);
  font-size: var(--font-size-xs);
}

/* 未保存の印（小さな点） */
.compose-dirty {
  width: var(--s-2);
  height: var(--s-2);
  border-radius: var(--r-full);
  background: var(--mute);
}

.spacer {
  flex: 1;
}

/* 本文エディタの最低高さ（約 10 行）。余った高さは広がるが、足りなくても 240px 未満には縮めない。 */
.compose-editor {
  flex: 1 0 240px;
  min-height: 240px;
}

.compose-error {
  margin: 0;
  padding: var(--s-2) var(--s-3);
  border: 1px solid var(--danger);
  border-radius: var(--r-sm);
  color: var(--danger);
  font-size: var(--font-size-sm);
}
</style>
