<script setup lang="ts">
import { MagicWand, WarningCircle } from '@iconoir/vue'
import { computed, onBeforeUnmount, onMounted, watch } from 'vue'
import { useAiDraft } from '../../../composables/useAiDraft'
import { instructionLength, MAX_INSTRUCTION_CHARS } from '../../../lib/aiInstruction'
import { common, compose as msg, settings as settingsMsg } from '../../../lib/messages'
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

// 設定が未保存のあいだは、既定の ChatGPT プランをログイン済みとして扱わない（案内文の出し分けだけに使う）。
const usingChatGPT = computed(() => settings.aiConfig?.configured === true && settings.aiConfig.provider === 'chatgpt')
const needText = computed(() => (usingChatGPT.value ? msg.ai.needLogin : msg.ai.needKey))

const setupOpen = computed(() => ai.step.value === 'setup')
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
      <BaseButton size="sm"
        v-if="!generating"
        variant="secondary"
        :disabled="ai.needsKey.value || busy"
        :title="ai.needsKey.value ? needText : undefined"
        @click="ai.start()"
      >
        <MagicWand class="icon icon-sm" aria-hidden="true" /> {{ msg.ai.button }}
      </BaseButton>
      <template v-else>
        <span class="ai-progress" role="status"><Spinner /> {{ msg.ai.generating }}</span>
        <BaseButton size="sm" variant="secondary" @click="ai.cancel()">{{ msg.ai.cancel }}</BaseButton>
      </template>
      <template v-if="ai.needsKey.value">
        <span class="ai-note">{{ needText }}</span>
        <BaseButton size="sm" variant="ghost" @click="ui.openSettings()">{{ msg.ai.openSettings }}</BaseButton>
      </template>
    </div>
    <AiDraftInstruction v-model="compose.aiInstruction" :disabled="generating" />
    <!-- 利用枠の案内もアラートの仲間なので、出現・消去は .ai-error と同じフェード（ガイド M5） -->
    <Transition name="alert">
      <UsageLimitNotice
        v-if="ai.usageLimit.value"
        @manage="settings.openExternal(settingsMsg.ai.chatgpt.usageUrl)"
        @dismiss="ai.usageLimit.value = false"
      />
    </Transition>
    <!-- インラインアラートの出現はフェードイン・消去はフェードアウト（ガイド M5） -->
    <Transition name="alert">
      <div v-if="!ai.usageLimit.value && ai.error.value" class="ai-error" role="alert">
        <WarningCircle class="icon ai-error-icon" aria-hidden="true" />
        <div class="ai-error-main">
          <strong class="ai-error-title">{{ common.alertTitle }}</strong>
          <p class="ai-error-body">{{ msg.ai.failed }}（{{ ai.error.value }}）</p>
        </div>
      </div>
    </Transition>

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

    <BaseDialog
      :open="replaceOpen"
      :title="msg.ai.replaceTitle"
      :body-label="msg.ai.replaceTitle"
      size="sm"
      @update:open="(v) => !v && ai.answerReplace(false)"
    >
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
  gap: var(--s-2);
}

.ai-row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--s-2);
}

.ai-progress {
  display: inline-flex;
  align-items: center;
  gap: var(--s-2);
  color: var(--mute);
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}

.ai-note {
  color: var(--ink);
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}

.ai-error {
  display: flex;
  align-items: flex-start;
  gap: var(--s-3);
  padding: var(--s-3) var(--s-4);
  border: 1px solid var(--danger-border);
  border-radius: var(--r-md);
  background: var(--danger-soft);
  color: var(--ink);
}

.ai-error-icon {
  flex: none;
  margin-top: var(--s-1);
  color: var(--danger);
}

.ai-error-main {
  flex: 1;
  min-width: 0;
}

.ai-error-title {
  display: block;
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
  font-weight: 600;
}

.ai-error-body {
  margin: 0;
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
  overflow-wrap: anywhere;
}

.dlg-text {
  margin: 0;
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}

.alert-enter-active {
  animation: ui-fade-in var(--motion-fade-in) var(--ease-out);
}

.alert-leave-active {
  animation: ui-fade-out var(--motion-fade-out) var(--ease-in) forwards;
}
</style>
