<script setup lang="ts">
import { agent } from '../../lib/messages'
import BaseButton from '../ui/BaseButton.vue'
import BaseDialog from '../ui/BaseDialog.vue'

// エージェントモードをオンにする同意の画面。送信先（プロバイダと URL）を表示する。同意の保存・そのあとの処理は呼び出し側。
// 作りは AIConsentDialog（「AI で下書き」の同意）と同じで、文言だけが違う（新着メールが自動で送られる）。
defineProps<{
  open: boolean
  providerLabel: string
  url: string
  /** 同意を保存している間（二重に押せないようにする）。 */
  busy?: boolean
}>()
const emit = defineEmits<{ agree: []; cancel: [] }>()
</script>

<template>
  <BaseDialog
    :open="open"
    :title="agent.consent.title"
    :body-label="agent.consent.title"
    size="sm"
    :busy="busy"
    @update:open="(v) => !v && emit('cancel')"
  >
    <div class="body" data-testid="agent-consent-body">
      <p class="text">{{ agent.consent.body(providerLabel, url) }}</p>
      <p class="text">{{ agent.consent.draft }}</p>
      <p class="text">{{ agent.consent.off }}</p>
    </div>
    <template #footer>
      <BaseButton :disabled="busy" data-testid="agent-consent-cancel" @click="emit('cancel')">
        {{ agent.consent.cancel }}
      </BaseButton>
      <BaseButton variant="primary" :loading="busy" data-testid="agent-consent-agree" @click="emit('agree')">
        {{ agent.consent.agree }}
      </BaseButton>
    </template>
  </BaseDialog>
</template>

<style scoped>
.body {
  display: flex;
  flex-direction: column;
  gap: var(--s-3);
}

.text {
  margin: 0;
  font-size: var(--font-size-base);
  line-height: var(--line-height-base);
}
</style>
