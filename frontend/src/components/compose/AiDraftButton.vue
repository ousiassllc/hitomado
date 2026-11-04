<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, watch } from 'vue'
import { useAiDraft } from '../../composables/useAiDraft'
import { compose as msg, settings as settingsMsg } from '../../lib/messages'
import { hasUserBody } from '../../lib/signature'
import { useComposeStore } from '../../stores/compose'
import { useSettingsStore } from '../../stores/settings'
import { useUiStore } from '../../stores/ui'
import { aiDestination } from '../settings/ai/aiDestination'
import BaseButton from '../ui/BaseButton.vue'
import BaseDialog from '../ui/BaseDialog.vue'
import Spinner from '../ui/Spinner.vue'
import UsageLimitNotice from './UsageLimitNotice.vue'

const props = defineProps<{ sessionId: string }>()
const emit = defineEmits<{ generated: [bodyHtml: string] }>()

const settings = useSettingsStore()
const compose = useComposeStore()
const ui = useUiStore()

onMounted(() => {
  if (settings.aiConfig === undefined) void settings.load()
})

const ai = useAiDraft({
  config: () => settings.aiConfig,
  hasBody: () => hasUserBody(compose.content.bodyHtml),
  recordConsent: (provider) => settings.recordAIConsent(provider),
  generate: (requestId) => settings.generateReplyDraft(props.sessionId, requestId),
  cancel: (requestId) => settings.cancelGenerate(requestId),
  onGenerated: (bodyHtml) => emit('generated', bodyHtml),
  newRequestId: () => crypto.randomUUID(),
})

const destination = computed(() => {
  const c = settings.aiConfig
  return c ? aiDestination(c.provider, c.baseUrl) : undefined
})

const usingChatGPT = computed(() => settings.aiConfig?.provider === 'chatgpt')
const needText = computed(() => (usingChatGPT.value ? msg.ai.needLogin : msg.ai.needKey))

const consentOpen = computed(() => ai.step.value === 'consent')
const replaceOpen = computed(() => ai.step.value === 'replace')
const generating = computed(() => ai.step.value === 'generating')
watch(generating, (v) => (compose.aiGenerating = v), { immediate: true })
onBeforeUnmount(() => (compose.aiGenerating = false))
const busy = computed(() => compose.sending || compose.preview !== undefined)
</script>

<template>
  <div class="ai-draft">
    <div class="ai-row">
      <BaseButton
        v-if="!generating"
        variant="secondary"
        :disabled="ai.needsKey.value || busy"
        :title="ai.needsKey.value ? needText : undefined"
        @click="ai.start()"
      >
        ✨ {{ msg.ai.button }}
      </BaseButton>
      <template v-else>
        <span class="ai-progress" role="status"><Spinner :size="14" /> {{ msg.ai.generating }}</span>
        <BaseButton variant="secondary" @click="ai.cancel()">{{ msg.ai.cancel }}</BaseButton>
      </template>
      <template v-if="ai.needsKey.value">
        <span class="ai-note">{{ needText }}</span>
        <BaseButton variant="ghost" @click="ui.openSettings()">{{ msg.ai.openSettings }}</BaseButton>
      </template>
      <template v-else-if="usingChatGPT">
        <span class="ai-note" data-testid="ai-using-chatgpt">{{ msg.ai.usingChatGPT }}</span>
        <BaseButton variant="ghost" @click="settings.openExternal(settingsMsg.ai.chatgpt.usageUrl)">
          {{ msg.ai.manageUsage }}
        </BaseButton>
      </template>
    </div>
    <UsageLimitNotice
      v-if="ai.usageLimit.value"
      @manage="settings.openExternal(settingsMsg.ai.chatgpt.usageUrl)"
      @dismiss="ai.usageLimit.value = false"
    />
    <p v-else-if="ai.error.value" class="ai-error" role="alert">{{ msg.ai.failed }}（{{ ai.error.value }}）</p>

    <BaseDialog
      :open="consentOpen"
      :title="msg.ai.consentTitle"
      size="sm"
      :closable="false"
    >
      <p v-if="destination" class="dlg-text">
        {{ msg.ai.consentBody(destination.providerLabel, destination.url) }}
      </p>
      <template #footer>
        <BaseButton variant="secondary" @click="ai.answerConsent(false)">{{ msg.ai.consentCancel }}</BaseButton>
        <BaseButton variant="primary" @click="ai.answerConsent(true)">{{ msg.ai.consentAgree }}</BaseButton>
      </template>
    </BaseDialog>

    <BaseDialog :open="replaceOpen" :title="msg.ai.replaceTitle" size="sm" :closable="false">
      <p class="dlg-text">{{ msg.ai.replaceBody }}</p>
      <template #footer>
        <BaseButton variant="secondary" @click="ai.answerReplace(false)">{{ msg.ai.replaceCancel }}</BaseButton>
        <BaseButton variant="primary" @click="ai.answerReplace(true)">{{ msg.ai.replaceOk }}</BaseButton>
      </template>
    </BaseDialog>
  </div>
</template>

<style scoped>
.ai-draft {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.ai-row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-2);
}

.ai-progress {
  display: inline-flex;
  align-items: center;
  gap: var(--space-2);
  color: var(--color-text-muted);
  font-size: var(--font-size-sm);
}

.ai-note {
  color: var(--color-text-muted);
  font-size: var(--font-size-xs);
}

.ai-error {
  margin: 0;
  padding: var(--space-2) var(--space-3);
  border: 1px solid var(--color-danger);
  border-radius: var(--radius-sm);
  color: var(--color-danger);
  font-size: var(--font-size-sm);
}

.dlg-text {
  margin: 0;
  font-size: var(--font-size-md);
  line-height: 1.6;
}
</style>
