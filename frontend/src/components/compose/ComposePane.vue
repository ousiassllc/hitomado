<script setup lang="ts">
import { computed, ref } from 'vue'
import { compose as msg } from '../../lib/messages'
import { useAccountsStore } from '../../stores/accounts'
import { useComposeStore } from '../../stores/compose'
import { useLayoutStore } from '../../stores/layout'
import BaseButton from '../ui/BaseButton.vue'
import AiDraftButton from './AiDraftButton.vue'
import AttachmentBar from './AttachmentBar.vue'
import RecipientField from './RecipientField.vue'
import RichTextEditor from './RichTextEditor.vue'
import SenderField from './SenderField.vue'
import SignatureConflictDialog from './SignatureConflictDialog.vue'

const compose = useComposeStore()
const accounts = useAccountsStore()
const layout = useLayoutStore()

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

const ccOpen = ref(false)
const bccOpen = ref(false)
const showCc = computed(() => ccOpen.value || compose.content.cc.length > 0)
const showBcc = computed(() => bccOpen.value || compose.content.bcc.length > 0)

function onKeydown(e: KeyboardEvent): void {
  // Ctrl+Enter は送信確認を開く（確認なしの送信はできない）
  if (e.key === 'Enter' && (e.ctrlKey || e.metaKey) && !e.isComposing) {
    e.preventDefault()
    send(false)
  }
}

const toField = ref<InstanceType<typeof RecipientField>>()
const ccField = ref<InstanceType<typeof RecipientField>>()
const bccField = ref<InstanceType<typeof RecipientField>>()

/** 入力途中の宛先を確定してから送信確認を開く（確定後の content を使う）。 */
function send(archive: boolean): void {
  if (!compose.canSend || frozen.value) return
  for (const f of [toField, ccField, bccField]) f.value?.commitPending()
  void compose.prepareSend(archive)
}
</script>

<template>
  <section
    v-if="session"
    class="compose-pane"
    data-file-drop-target
    :aria-label="msg.paneLabel"
    @keydown="onKeydown"
  >
    <header class="compose-head">
      <h2 class="compose-title">{{ title }}</h2>
      <span v-if="compose.saving" class="compose-status" role="status">{{ msg.saving }}</span>
      <span v-else-if="compose.dirty" class="compose-status">●</span>
      <span class="spacer" />
      <BaseButton variant="ghost" :disabled="compose.sending" @click="compose.close()">{{ msg.close }}</BaseButton>
      <BaseButton variant="ghost" :disabled="compose.sending" @click="compose.discard()">{{ msg.discard }}</BaseButton>
    </header>

    <div class="compose-fields">
      <SenderField
        :from="session.from ?? undefined"
        :account-id="session.accountId"
        :locked="session.senderLocked"
        :accounts="accounts.accounts"
        @select="compose.setSender"
      />
      <div class="to-row">
        <RecipientField
          ref="toField"
          v-model="compose.content.to"
          class="to-field"
          :label="msg.fields.to"
          :account-id="session.accountId"
          :disabled="frozen"
        />
        <div class="toggles">
          <button v-if="!showCc" type="button" class="toggle" @click="ccOpen = true">{{ msg.fields.showCc }}</button>
          <button v-if="!showBcc" type="button" class="toggle" @click="bccOpen = true">{{ msg.fields.showBcc }}</button>
        </div>
      </div>
      <RecipientField
        v-if="showCc"
        ref="ccField"
        v-model="compose.content.cc"
        :label="msg.fields.cc"
        :account-id="session.accountId"
        :disabled="frozen"
      />
      <RecipientField
        v-if="showBcc"
        ref="bccField"
        v-model="compose.content.bcc"
        :label="msg.fields.bcc"
        :account-id="session.accountId"
        :disabled="frozen"
      />
      <label class="subject-row">
        <span class="subject-label">{{ msg.fields.subject }}</span>
        <input
          v-model="compose.content.subject"
          class="subject-input"
          type="text"
          :placeholder="msg.fields.subjectPlaceholder"
          :disabled="frozen"
        />
      </label>
    </div>

    <AiDraftButton
      v-if="canAi"
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
      <span v-else-if="!layout.isStack" class="foot-hint">{{ msg.shortcutHint }}</span>
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
      <BaseButton
        variant="primary"
        :disabled="!compose.canSend || frozen"
        :loading="compose.preparing || compose.sending"
        @click="send(false)"
      >
        {{ compose.sending ? msg.sending : msg.send }}
      </BaseButton>
    </footer>

    <SignatureConflictDialog />
  </section>
</template>

<style scoped>
.compose-pane {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  min-height: 0;
  padding: var(--space-4);
  background: var(--color-bg);
  color: var(--color-text);
}

.compose-pane.file-drop-target-active {
  outline: 2px dashed var(--color-accent);
  outline-offset: -4px;
}

.compose-head,
.compose-foot {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}

.compose-title {
  margin: 0;
  font-size: var(--font-size-lg);
}

.compose-status,
.foot-hint {
  color: var(--color-text-muted);
  font-size: var(--font-size-xs);
}

.spacer {
  flex: 1;
}

.compose-fields {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.to-row {
  display: flex;
  align-items: flex-start;
  gap: var(--space-2);
}

.to-field {
  flex: 1;
  min-width: 0;
}

.toggles {
  display: flex;
  gap: var(--space-1);
  padding-top: 4px;
}

.toggle {
  padding: 2px var(--space-2);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--color-text-muted);
  font-size: var(--font-size-xs);
  cursor: pointer;
}

.toggle:hover {
  background: var(--color-selected);
}

.subject-row {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}

.subject-label {
  flex: 0 0 56px;
  color: var(--color-text-muted);
  font-size: var(--font-size-sm);
}

.subject-input {
  flex: 1;
  height: 34px;
  padding: 0 var(--space-2);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-sm);
  background: var(--color-bg);
  color: var(--color-text);
  font-size: var(--font-size-md);
}

.subject-input:focus {
  border-color: var(--color-focus);
  outline: none;
}

.compose-editor {
  flex: 1;
  min-height: 200px;
}

.compose-error {
  margin: 0;
  padding: var(--space-2) var(--space-3);
  border: 1px solid var(--color-danger);
  border-radius: var(--radius-sm);
  color: var(--color-danger);
  font-size: var(--font-size-sm);
}
</style>
