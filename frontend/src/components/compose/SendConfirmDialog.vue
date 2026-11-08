<script setup lang="ts">
import { WarningTriangle } from '@iconoir/vue'
import { computed, ref, watch } from 'vue'
import { compose as msg } from '../../lib/messages'
import { addressLabel } from '../../lib/recipients'
import { useComposeStore } from '../../stores/compose'
import SenderBadge from '../account/SenderBadge.vue'
import BaseButton from '../ui/BaseButton.vue'
import BaseDialog from '../ui/BaseDialog.vue'

const compose = useComposeStore()

// 開閉は compose.preview で決め、表示する中身は最後の preview を保持する。
// キャンセル・送信完了で preview が消えても、退出のフェード（240ms）の間は中身を変えない（ガイド M5）。
// previewArchive は clearSession で戻るので、preview と同時に控える。
const isOpen = computed(() => compose.preview !== undefined)
const shown = ref(compose.preview ? { preview: compose.preview, archive: compose.previewArchive } : undefined)
watch(
  () => compose.preview,
  (p) => {
    if (p) shown.value = { preview: p, archive: compose.previewArchive }
  },
)
const preview = computed(() => shown.value?.preview)
const warnings = computed(() => preview.value?.warnings ?? [])
const recipients = computed(() => {
  const p = preview.value
  if (!p) return []
  return [
    { key: 'to', label: msg.confirm.to, list: p.to ?? [] },
    { key: 'cc', label: msg.confirm.cc, list: p.cc ?? [] },
    { key: 'bcc', label: msg.confirm.bcc, list: p.bcc ?? [] },
  ].filter((r) => r.list.length > 0)
})
const sendLabel = computed(() => {
  const email = preview.value?.from.email ?? ''
  return shown.value?.archive ? msg.confirm.sendFromArchive(email) : msg.confirm.sendFrom(email)
})

function onOpen(open: boolean): void {
  if (!open) compose.cancelPreview()
}
</script>

<template>
  <BaseDialog
    :open="isOpen"
    :title="msg.confirm.title"
    :description="msg.confirm.description"
    size="lg"
    :busy="compose.sending"
    @update:open="onOpen"
  >
    <div v-if="preview" class="confirm">
      <section class="confirm-from" :aria-label="msg.confirm.fromLabel">
        <span class="confirm-from-label">{{ msg.confirm.fromLabel }}</span>
        <SenderBadge :from="preview.from" :color="preview.from.color" />
      </section>

      <ul v-if="warnings.length > 0" class="warnings" :aria-label="msg.confirm.warnings" role="alert">
        <li
          v-for="w in warnings"
          :key="w.code"
          class="warning"
          :class="{ 'warning-strong': w.code === 'OTHER_OWN_ACCOUNT' }"
        >
          <WarningTriangle class="icon icon-sm warning-icon" aria-hidden="true" />{{ w.message }}
          <span v-if="w.emails && w.emails.length > 0" class="warning-emails">{{ w.emails.join('、') }}</span>
        </li>
      </ul>

      <dl class="confirm-fields">
        <template v-for="r in recipients" :key="r.key">
          <dt>{{ r.label }}</dt>
          <dd>{{ r.list.map(addressLabel).join('、') }}</dd>
        </template>
        <dt>{{ msg.confirm.subject }}</dt>
        <dd>{{ preview.subject || msg.confirm.emptySubject }}</dd>
        <dt>{{ msg.confirm.attachments }}</dt>
        <dd>{{ msg.confirm.attachmentCount(preview.attachmentCount) }}</dd>
        <dt>{{ msg.confirm.bodyPreview }}</dt>
        <!-- biome-ignore lint/a11y/noNoninteractiveTabindex lint/a11y/useSemanticElements: スクロール領域は子にフォーカス可能な要素が無く、Tab で入れないとキーボードで動かせない。ガイド B6 の指定（tabindex=0・role=region・領域名）。dd は dl の中なので section には変えられない -->
        <dd class="confirm-body" tabindex="0" role="region" :aria-label="msg.confirm.bodyPreview">{{ preview.bodyPreview || msg.confirm.emptyBody }}</dd>
      </dl>
      <p v-if="shown?.archive" class="confirm-note">{{ msg.confirm.archiveNote }}</p>
      <p v-if="compose.error" class="confirm-error" role="alert">{{ compose.error }}</p>
    </div>
    <template #footer>
      <BaseButton variant="secondary" :disabled="compose.sending" @click="compose.cancelPreview()">
        {{ msg.confirm.cancel }}
      </BaseButton>
      <BaseButton variant="primary" :loading="compose.sending" @click="compose.confirmSend()">
        {{ compose.sending ? msg.confirm.sendingNow : sendLabel }}
      </BaseButton>
    </template>
  </BaseDialog>
</template>

<style scoped>
.confirm {
  display: flex;
  flex-direction: column;
  gap: var(--s-3);
}

.confirm-from {
  display: flex;
  flex-direction: column;
  gap: var(--gap-label);
}

.confirm-from-label {
  color: var(--mute);
  font-size: var(--font-size-xs);
}

.warnings {
  display: flex;
  flex-direction: column;
  gap: var(--s-2);
  margin: 0;
  padding: 0;
  list-style: none;
}

.warning {
  padding: var(--s-3) var(--s-4);
  border-radius: var(--r-md);
  border: 1px solid var(--warning-border);
  background: var(--warning-soft);
  color: var(--ink);
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}

.warning-icon {
  color: var(--warning);
}

/* 他の自アカウント宛は、送信元より目立たせる（状態色の細枠・薄い地・大きめの文字。太字にしない）。 */
.warning-strong {
  border-color: var(--danger-border);
  background: var(--danger-soft);
  font-size: var(--font-size-lg);
  line-height: var(--line-height-lg);
}

.warning-strong .warning-icon {
  color: var(--danger);
}

.warning-emails {
  display: block;
  font-weight: 400;
  overflow-wrap: anywhere;
}

.confirm-fields {
  display: grid;
  grid-template-columns: max-content 1fr;
  gap: var(--s-2) var(--s-3);
  margin: 0;
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}

.confirm-fields dt {
  color: var(--mute);
  font-size: var(--font-size-sm);
}

.confirm-fields dd {
  margin: 0;
  overflow-wrap: anywhere;
}

.confirm-body {
  max-height: min(16em, 40vh);
  overflow-y: auto;
  white-space: pre-wrap;
}

.confirm-note {
  margin: 0;
  color: var(--mute);
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}

.confirm-error {
  margin: 0;
  color: var(--danger);
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}
</style>
