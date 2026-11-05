<script setup lang="ts">
import { computed } from 'vue'
import { compose as msg } from '../../lib/messages'
import { addressLabel } from '../../lib/recipients'
import { useComposeStore } from '../../stores/compose'
import SenderBadge from '../account/SenderBadge.vue'
import BaseButton from '../ui/BaseButton.vue'
import BaseDialog from '../ui/BaseDialog.vue'

const compose = useComposeStore()

const preview = computed(() => compose.preview)
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
  return compose.previewArchive ? msg.confirm.sendFromArchive(email) : msg.confirm.sendFrom(email)
})

function onOpen(open: boolean): void {
  if (!open) compose.cancelPreview()
}
</script>

<template>
  <BaseDialog
    :open="preview !== undefined"
    :title="msg.confirm.title"
    :description="msg.confirm.description"
    size="lg"
    :closable="!compose.sending"
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
          <strong v-if="w.code === 'OTHER_OWN_ACCOUNT'">⚠ </strong>{{ w.message }}
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
        <dd class="confirm-body">{{ preview.bodyPreview || msg.confirm.emptyBody }}</dd>
      </dl>
      <p v-if="compose.previewArchive" class="confirm-note">{{ msg.confirm.archiveNote }}</p>
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
  gap: var(--space-3);
}

.confirm-from {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
}

.confirm-from-label {
  color: var(--color-text-muted);
  font-size: var(--font-size-xs);
}

.warnings {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  margin: 0;
  padding: 0;
  list-style: none;
}

.warning {
  padding: var(--space-2) var(--space-3);
  border-radius: var(--radius-sm);
  background: var(--color-warning-bg);
  color: var(--color-warning-text);
  font-size: var(--font-size-sm);
}

.warning-strong {
  border: 2px solid var(--color-danger);
  font-size: var(--font-size-md);
  font-weight: 700;
}

.warning-emails {
  display: block;
  font-weight: 400;
  overflow-wrap: anywhere;
}

.confirm-fields {
  display: grid;
  grid-template-columns: max-content 1fr;
  gap: var(--space-2) var(--space-3);
  margin: 0;
  font-size: var(--font-size-md);
}

.confirm-fields dt {
  color: var(--color-text-muted);
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
  color: var(--color-text-muted);
  font-size: var(--font-size-sm);
}

.confirm-error {
  margin: 0;
  color: var(--color-danger);
  font-size: var(--font-size-sm);
}
</style>
