<script setup lang="ts">
import { compose as msg } from '../../../lib/messages'
import BaseButton from '../../ui/BaseButton.vue'
import BaseDialog from '../../ui/BaseDialog.vue'

// AI プロバイダへの送信の同意の画面（F-08）。送信先（プロバイダと URL）を表示する。
// 「AI で下書き」（作成画面）と設定画面のプロバイダ選択の両方で、この 1 つを使う。同意の保存・そのあとの処理は呼び出し側。
defineProps<{
  open: boolean
  providerLabel: string
  url: string
  agreeLabel: string
  /** 同意を保存している間（二重に押せないようにする）。 */
  busy?: boolean
}>()
const emit = defineEmits<{ agree: []; cancel: [] }>()
</script>

<template>
  <BaseDialog
    :open="open"
    :title="msg.ai.consentTitle"
    :body-label="msg.ai.consentTitle"
    size="sm"
    :busy="busy"
    @update:open="(v) => !v && emit('cancel')"
  >
    <p class="text" data-testid="ai-consent-body">{{ msg.ai.consentBody(providerLabel, url) }}</p>
    <template #footer>
      <BaseButton variant="secondary" :disabled="busy" data-testid="ai-consent-cancel" @click="emit('cancel')">
        {{ msg.ai.consentCancel }}
      </BaseButton>
      <BaseButton variant="primary" :loading="busy" data-testid="ai-consent-agree" @click="emit('agree')">
        {{ agreeLabel }}
      </BaseButton>
    </template>
  </BaseDialog>
</template>

<style scoped>
.text {
  margin: 0;
  font-size: var(--font-size-base);
  line-height: var(--line-height-base);
}
</style>
