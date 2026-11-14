<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { parseBindingError } from '../../lib/api'
import { agent } from '../../lib/messages'
import { useAgentStore } from '../../stores/agent'
import { usePlatformStore } from '../../stores/platform'
import { useSettingsStore } from '../../stores/settings'
import { useUiStore } from '../../stores/ui'
import { aiDestination, DEFAULT_AI_PROVIDER } from '../settings/ai/aiDestination'
import BaseButton from '../ui/BaseButton.vue'
import BaseDialog from '../ui/BaseDialog.vue'
import BaseSwitch from '../ui/BaseSwitch.vue'
import AgentConsentDialog from './AgentConsentDialog.vue'

// 設定の「エージェント」ページ。エージェントモード（既定オフ）と、マスコット（Windows のみ）のスイッチ。
// オンにするときは、新着メールが AI へ自動で送られることへの同意を取る。AI が使えない状態では同意の代わりに AI の設定へ案内する。
const msg = agent.settings
const store = useAgentStore()
const settings = useSettingsStore()
const platform = usePlatformStore()
const ui = useUiStore()

/** 開いたとき最新の状態を読み直す（AI の設定を直したあとで AI が使えるようになっていることがある）。 */
onMounted(() => void store.load())

const consentOpen = ref(false)
const needAiOpen = ref(false)
const saving = ref(false)

const destination = computed(() =>
  aiDestination(settings.aiConfig?.provider ?? DEFAULT_AI_PROVIDER, settings.aiConfig?.baseUrl),
)

async function toggle(on: boolean): Promise<void> {
  if (!on) {
    try {
      await store.setEnabled(false, false)
    } catch (e) {
      ui.pushAlert(parseBindingError(e).message)
    }
    return
  }
  if (store.settings.aiReady) consentOpen.value = true
  else needAiOpen.value = true
}

async function agree(): Promise<void> {
  if (saving.value) return
  saving.value = true
  try {
    await store.setEnabled(true, true)
    consentOpen.value = false
  } catch (e) {
    const err = parseBindingError(e)
    consentOpen.value = false
    if (err.code === 'AI_NOT_CONFIGURED' || err.code === 'AI_CONSENT_REQUIRED') needAiOpen.value = true
    else ui.pushAlert(err.message)
  } finally {
    saving.value = false
  }
}

function goAiSettings(): void {
  needAiOpen.value = false
  ui.openSettings('ai')
}

async function toggleMascot(on: boolean): Promise<void> {
  try {
    await store.setMascot(on)
  } catch (e) {
    ui.pushAlert(parseBindingError(e).message)
  }
}
</script>

<template>
  <section class="agent" :aria-label="msg.tab">
    <div class="block" data-testid="agent-enabled">
      <div class="toggle">
        <BaseSwitch :model-value="store.enabled" :ariaLabel="msg.switchLabel" @update:model-value="toggle" />
        <strong>{{ msg.heading }}</strong>
      </div>
      <p class="body">{{ msg.description }}</p>
      <p class="hint">{{ msg.scope }}</p>
      <div v-if="store.enabled && !store.settings.aiReady" class="notice" role="status" data-testid="agent-ai-not-ready">
        <p class="body">{{ msg.aiNotReady }}</p>
        <div>
          <BaseButton size="sm" data-testid="agent-ai-settings" @click="ui.openSettings('ai')">{{ msg.openAiSettings }}</BaseButton>
        </div>
      </div>
    </div>

    <div v-if="platform.supportsMascot" class="block" data-testid="agent-mascot">
      <div class="toggle">
        <BaseSwitch
          :model-value="store.settings.mascotEnabled && store.enabled"
          :ariaLabel="msg.mascotSwitchLabel"
          :disabled="!store.enabled"
          @update:model-value="toggleMascot"
        />
        <strong>{{ msg.mascotHeading }}</strong>
      </div>
      <p class="body">{{ msg.mascotDescription }}</p>
      <p v-if="!store.enabled" class="hint" data-testid="agent-mascot-needs-agent">{{ msg.mascotNeedsAgent }}</p>
    </div>

    <AgentConsentDialog
      :open="consentOpen"
      :provider-label="destination.providerLabel"
      :url="destination.url"
      :busy="saving"
      @agree="agree"
      @cancel="consentOpen = false"
    />

    <BaseDialog
      :open="needAiOpen"
      :title="msg.needAiTitle"
      :body-label="msg.needAiTitle"
      size="sm"
      @update:open="(v) => (needAiOpen = v)"
    >
      <p class="body" data-testid="agent-need-ai-body">{{ msg.needAiBody }}</p>
      <template #footer>
        <BaseButton data-testid="agent-need-ai-cancel" @click="needAiOpen = false">{{ msg.needAiCancel }}</BaseButton>
        <BaseButton variant="primary" data-testid="agent-need-ai-open" @click="goAiSettings">{{ msg.needAiOpen }}</BaseButton>
      </template>
    </BaseDialog>
  </section>
</template>

<style scoped>
.agent {
  display: flex;
  flex-direction: column;
  gap: var(--gap-field);
}

.block {
  display: flex;
  flex-direction: column;
  gap: var(--s-2);
}

.toggle {
  display: flex;
  align-items: center;
  gap: var(--s-2);
}

.body,
.hint {
  margin: 0;
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}

.hint {
  color: var(--mute);
}

.notice {
  display: flex;
  flex-direction: column;
  gap: var(--s-2);
  padding: var(--s-3);
  border: 1px solid var(--border);
  border-radius: var(--r-md);
  background: var(--bone);
}
</style>
