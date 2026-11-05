<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useAiDraft } from '../../../composables/useAiDraft'
import { instructionLength, MAX_INSTRUCTION_CHARS } from '../../../lib/aiInstruction'
import { compose as msg, settings as settingsMsg } from '../../../lib/messages'
import { hasUserBody } from '../../../lib/signature'
import { useComposeStore } from '../../../stores/compose'
import { useSettingsStore } from '../../../stores/settings'
import { useUiStore } from '../../../stores/ui'
import AIConsentDialog from '../../settings/ai/AIConsentDialog.vue'
import AISetupDialog from '../../settings/ai/AISetupDialog.vue'
import { aiDestination } from '../../settings/ai/aiDestination'
import BaseButton from '../../ui/BaseButton.vue'
import BaseDialog from '../../ui/BaseDialog.vue'
import Spinner from '../../ui/Spinner.vue'
import AiDraftInstruction from './AiDraftInstruction.vue'
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
  // 同意は、保存済みの設定の送信先（OpenAI 互換は Base URL も）に対して記録する。
  recordConsent: (provider) => settings.recordAIConsent(provider, settings.aiConfig?.baseUrl ?? ''),
  generate: (requestId) => settings.generateReplyDraft(props.sessionId, requestId, compose.aiInstruction),
  // 上限を超えた指示は、生成を始めずにエラーを出す。
  instructionError: () =>
    instructionLength(compose.aiInstruction) > MAX_INSTRUCTION_CHARS ? msg.ai.instruction.tooLong(MAX_INSTRUCTION_CHARS) : undefined,
  cancel: (requestId) => settings.cancelGenerate(requestId),
  // 初回の案内から、設定画面の AI の流れへ進む。ChatGPT は同意→ログイン→モデルの選択、別のプロバイダはプロバイダの選択から。
  openSetup: (choice) => ui.openSettings('ai', choice === 'chatgpt' ? 'chatgpt-login' : undefined),
  onGenerated: (bodyHtml) => emit('generated', bodyHtml),
  newRequestId: () => crypto.randomUUID(),
})

const destination = computed(() => {
  const c = settings.aiConfig
  return c ? aiDestination(c.provider, c.baseUrl) : undefined
})

// 設定が未保存のあいだは、既定の ChatGPT プランを「使用中」とは表示しない。
const usingChatGPT = computed(() => settings.aiConfig?.configured === true && settings.aiConfig.provider === 'chatgpt')
const needText = computed(() => (usingChatGPT.value ? msg.ai.needLogin : msg.ai.needKey))

const setupOpen = computed(() => ai.step.value === 'setup')
const consentOpen = computed(() => ai.step.value === 'consent')
const replaceOpen = computed(() => ai.step.value === 'replace')
const generating = computed(() => ai.step.value === 'generating')
watch(generating, (v) => (compose.aiGenerating = v), { immediate: true })
onBeforeUnmount(() => (compose.aiGenerating = false))
const busy = computed(() => compose.sending || compose.preview !== undefined)

/** 指示の入力欄の開閉（初期は閉じる。値は閉じても作成セッションに残る）。 */
const instructionOpen = ref(false)
const instructionLabel = computed(() => {
  if (instructionOpen.value) return msg.ai.instruction.hide
  return compose.aiInstruction.trim() === '' ? msg.ai.instruction.add : msg.ai.instruction.edit
})
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
      <BaseButton
        variant="ghost"
        :aria-expanded="instructionOpen"
        data-testid="ai-instruction-toggle"
        @click="instructionOpen = !instructionOpen"
      >
        {{ instructionLabel }}
      </BaseButton>
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
    <AiDraftInstruction v-if="instructionOpen" v-model="compose.aiInstruction" :disabled="generating" />
    <UsageLimitNotice
      v-if="ai.usageLimit.value"
      @manage="settings.openExternal(settingsMsg.ai.chatgpt.usageUrl)"
      @dismiss="ai.usageLimit.value = false"
    />
    <p v-else-if="ai.error.value" class="ai-error" role="alert">{{ msg.ai.failed }}（{{ ai.error.value }}）</p>

    <AISetupDialog :open="setupOpen" @choose="ai.answerSetup($event)" />

    <AIConsentDialog
      v-if="destination"
      :open="consentOpen"
      :provider-label="destination.providerLabel"
      :url="destination.url"
      :agree-label="msg.ai.consentAgree"
      @agree="ai.answerConsent(true)"
      @cancel="ai.answerConsent(false)"
    />

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
