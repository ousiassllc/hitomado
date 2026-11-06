<script setup lang="ts">
import { settings as msg } from '../../../lib/messages'
import BaseButton from '../../ui/BaseButton.vue'
import AIConsentDialog from './AIConsentDialog.vue'
import type { AIDestination } from './aiDestination'

// 設定画面の「送信への同意」。選んだ送信先の同意の状態と、同意の画面（F-08）を開くボタン。同意の保存・そのあとの取得は親（AISettings）。
defineProps<{
  text: string
  error?: string
  /** まだ同意していない（ボタンを出す）。 */
  needed: boolean
  disabled: boolean
  destination: AIDestination
  /** 同意の画面が開いているか／同意を保存している間。 */
  open: boolean
  saving: boolean
}>()
const emit = defineEmits<{ ask: []; agree: []; decline: [] }>()
</script>

<template>
  <div class="consent">
    <div class="group">
      <h4 class="sub">{{ msg.ai.consentHeading }}</h4>
      <p class="status" role="status">{{ text }}</p>
    </div>
    <p v-if="error" class="error" role="alert">{{ error }}</p>
    <div v-if="needed" class="actions">
      <BaseButton variant="secondary" :disabled="disabled" data-testid="ai-consent-now" @click="emit('ask')">
        {{ msg.ai.consentAgreeNow(msg.ai.destination(destination.providerLabel, destination.url)) }}
      </BaseButton>
    </div>
    <AIConsentDialog
      :open="open"
      :provider-label="destination.providerLabel"
      :url="destination.url"
      :agree-label="msg.ai.consentDialogAgree"
      :busy="saving"
      @agree="emit('agree')"
      @cancel="emit('decline')"
    />
  </div>
</template>

<style scoped>
.consent {
  display: flex;
  flex-direction: column;
  gap: var(--gap-field);
}

/* 見出しとその状態文の組。組の中は --s-2、組どうしは親の gap（--gap-field）。 */
.group {
  display: flex;
  flex-direction: column;
  gap: var(--s-2);
}

.sub {
  margin: 0;
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}

.status {
  margin: 0;
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}

.error {
  margin: 0;
  color: var(--danger);
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}

.actions {
  display: flex;
  justify-content: flex-end;
  gap: var(--s-2);
}
</style>
