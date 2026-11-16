<script setup lang="ts">
import { InfoCircle, MagicWand, WarningCircle } from '@iconoir/vue'
import { computed, nextTick, onBeforeUnmount, onMounted, ref, useId, watch } from 'vue'
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
import { draftPrecheckConfig } from '../../settings/ai/aiPrecheckConfig'
import BaseButton from '../../ui/BaseButton.vue'
import BaseDialog from '../../ui/BaseDialog.vue'
import Spinner from '../../ui/Spinner.vue'
import AiDraftInstruction from './AiDraftInstruction.vue'
import UsageLimitNotice from './UsageLimitNotice.vue'

const props = defineProps<{
  sessionId: string
  /** 新規作成の下書き（#365）。スレッドが無いので、AI への指示が必須で、入力済みの件名・宛先も渡す。返信は従来どおり。 */
  newMail?: boolean
}>()
const emit = defineEmits<{ generated: [bodyHtml: string] }>()

const settings = useSettingsStore()
const compose = useComposeStore()
const ui = useUiStore()

onMounted(() => {
  if (settings.aiConfig === undefined) void settings.load()
})

const ai = useAiDraft({
  config: () => draftPrecheckConfig(settings.aiConfig),
  hasBody: () => hasUserBody(compose.content.bodyHtml),
  // 同意は、保存済みの設定の送信先（OpenAI 互換は Base URL も）に対して記録する。
  recordConsent: (provider) => settings.recordAIConsent(provider, settings.aiConfig?.baseUrl ?? ''),
  generate: (requestId) =>
    props.newMail
      ? settings.generateNewDraft(props.sessionId, requestId, compose.aiInstruction, {
          subject: compose.content.subject,
          to: compose.content.to,
        })
      : settings.generateReplyDraft(props.sessionId, requestId, compose.aiInstruction),
  // 新規作成は指示が必須。空のまま押したら、生成も同意の確認も始めず、指示を書くよう案内する。
  instructionMissing: () => props.newMail === true && instructionLength(compose.aiInstruction) === 0,
  // 上限を超えた指示は、生成を始めずにエラーを出す。
  instructionError: () =>
    instructionLength(compose.aiInstruction) > MAX_INSTRUCTION_CHARS ? msg.ai.instruction.tooLong(MAX_INSTRUCTION_CHARS) : undefined,
  cancel: (requestId) => settings.cancelGenerate(requestId),
  // 初回の案内から、設定画面の AI の流れへ進む。ChatGPT は同意→ログイン→モデルの選択、別のプロバイダはプロバイダの選択から。
  openSetup: (choice) => ui.openSettings('ai', choice === 'chatgpt' ? 'chatgpt-login' : undefined),
  onGenerated: (bodyHtml) => emit('generated', bodyHtml),
  newRequestId: () => crypto.randomUUID(),
})

const instructionField = ref<InstanceType<typeof AiDraftInstruction>>()
watch(
  () => ai.needsInstruction.value,
  async (needed) => {
    if (!needed) return
    await nextTick()
    instructionField.value?.focus()
  },
)
// 指示が入ったら案内は役目を終える。
watch(
  () => instructionLength(compose.aiInstruction),
  (n) => {
    if (n > 0) ai.needsInstruction.value = false
  },
)

const destination = computed(() => {
  const c = settings.aiConfig
  return c ? aiDestination(c.features.draft.provider, c.baseUrl) : undefined
})

// 「AI で下書き」に選んだのが ChatGPT プランのときだけ、ログインの案内にする（案内文の出し分けだけに使う）。
const usingChatGPT = computed(() => settings.aiConfig?.features.draft.provider === 'chatgpt')
const needText = computed(() => (usingChatGPT.value ? msg.ai.needLogin : msg.ai.needKey))
// 無効なボタンの理由は title（OS 標準のツールチップ）ではなく、見えている .ai-note をボタンにつなぐ（#264）。
const noteId = useId()

const setupOpen = computed(() => ai.step.value === 'setup')
const consentOpen = computed(() => ai.step.value === 'consent')
const replaceOpen = computed(() => ai.step.value === 'replace')
const generating = computed(() => ai.step.value === 'generating')
watch(generating, (v) => (compose.aiGenerating = v), { immediate: true })
onBeforeUnmount(() => {
  ai.dispose() // 作成セッションを閉じた・破棄したら、進行中の生成を中止する（Go 側の呼び出しを走らせ続けない）
  compose.aiGenerating = false
})
const busy = computed(() => compose.sending || compose.preview !== undefined)
</script>

<template>
  <div class="ai-draft">
    <div class="ai-row">
      <BaseButton size="sm"
        v-if="!generating"
        variant="secondary"
        :disabled="ai.needsKey.value || busy"
        :aria-describedby="ai.needsKey.value ? noteId : undefined"
        @click="ai.start()"
      >
        <MagicWand class="icon icon-sm" aria-hidden="true" /> {{ msg.ai.button }}
      </BaseButton>
      <template v-else>
        <span class="ai-progress" role="status"><Spinner /> {{ msg.ai.generating }}</span>
        <BaseButton size="sm" variant="secondary" @click="ai.cancel()">{{ msg.ai.cancel }}</BaseButton>
      </template>
      <template v-if="ai.needsKey.value">
        <span :id="noteId" class="ai-note">{{ needText }}</span>
        <BaseButton size="sm" variant="ghost" @click="ui.openSettings()">{{ msg.ai.openSettings }}</BaseButton>
      </template>
    </div>
    <AiDraftInstruction ref="instructionField" v-model="compose.aiInstruction" :disabled="generating" :required="newMail" />
    <!-- 指示が空のまま押された案内（#365）。エラーではないので、お知らせ（info）の見た目。出現・消去は他のインラインアラートと同じフェード -->
    <Transition name="alert">
      <div v-if="ai.needsInstruction.value" class="ai-hint" role="status" data-testid="ai-instruction-needed">
        <InfoCircle class="icon ai-hint-icon" aria-hidden="true" />
        <div class="ai-hint-main">
          <strong class="ai-hint-title">{{ common.noticeTitle }}</strong>
          <p class="ai-hint-body">{{ msg.ai.instruction.needed }}</p>
        </div>
      </div>
    </Transition>
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

.ai-hint {
  display: flex;
  align-items: flex-start;
  gap: var(--s-3);
  padding: var(--s-3) var(--s-4);
  border: 1px solid var(--info-border);
  border-radius: var(--r-md);
  background: var(--info-soft);
  color: var(--ink);
}

.ai-hint-icon {
  flex: none;
  margin-top: var(--s-1);
  color: var(--info);
}

.ai-hint-main {
  flex: 1;
  min-width: 0;
}

.ai-hint-title {
  display: block;
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
  font-weight: 600;
}

.ai-hint-body {
  margin: 0;
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
  overflow-wrap: anywhere;
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
  font-size: var(--font-size-base);
  line-height: var(--line-height-base);
}

.alert-enter-active {
  animation: ui-fade-in var(--motion-fade-in) var(--ease-out);
}

.alert-leave-active {
  animation: ui-fade-out var(--motion-fade-out) var(--ease-in) forwards;
}
</style>
