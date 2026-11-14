<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { parseBindingError } from '../../../../lib/api'
import { inboxZero, settings as msg } from '../../../../lib/messages'
import { useInboxZeroStore } from '../../../../stores/inboxZero'
import { useUiStore } from '../../../../stores/ui'
import BaseSwitch from '../../../ui/BaseSwitch.vue'
import AIApiKey from '../AIApiKey.vue'
import AIConsentSection from '../AIConsentSection.vue'
import AIDestinationBox from '../AIDestinationBox.vue'
import { type AiBusy, useAiConnect } from '../useAiConnect'
import { INBOX_ZERO_REASONS, type InboxZeroReason } from './inboxZeroReasons'

// TypeSafe AI（Jev）の「つなぎ先」のページ（#314・#315・#376）。文章を書かないモデルなので、モデルの選択・エフォートは無く、
// 「AI で下書き」には選べない（Inbox Zero の「機能ごとに使う AI」にだけ選べる）。ここでは API キーの登録・削除と送信への同意を行う（キーは資格情報ストア）。
const PROVIDER = 'typesafe'
const ui = useUiStore()
const zero = useInboxZeroStore()
onMounted(() => {
  void zero.loadReasons()
  if (ui.takeAiConnectStart()) c.begin()
})

const provider = ref(PROVIDER)
const keyForm = ref<InstanceType<typeof AIApiKey>>()
const busy = ref<AiBusy>()
const errors = ref<{ key?: string; consent?: string; baseUrl?: string }>({})
const consentOpen = ref(false)
const label = msg.ai.providerOptions[PROVIDER] ?? PROVIDER

const c = useAiConnect({ provider, busy, errors, consentOpen, keyForm, chatgptLogin: ref(undefined) })

const reasonsError = ref<string>()

/** 理由のオン・オフ（#314）。切り替えたらすぐ保存する。保存に失敗したら画面の値は変えない。 */
async function toggleReason(reason: InboxZeroReason, on: boolean): Promise<void> {
  reasonsError.value = undefined
  try {
    await zero.setReasons({ ...zero.reasons, [reason]: on })
  } catch (e) {
    reasonsError.value = parseBindingError(e).message || inboxZero.settings.saveFailed
  }
}
</script>

<template>
  <section class="ai" :aria-label="label" data-testid="ai-typesafe-page">
    <div class="group">
      <h3 class="heading">{{ label }}</h3>
      <p class="intro">{{ msg.ai.typesafe.note }}</p>
    </div>
    <AIDestinationBox :provider="PROVIDER" :draft-provider="PROVIDER" draft-base-url="" />
    <AIApiKey
      ref="keyForm"
      :provider-label="label"
      :has-key="c.hasCredential.value"
      :busy="busy"
      :error="errors.key"
      @save="c.saveKey"
      @clear="c.clearKey"
    />
    <AIConsentSection
      :text="c.consentText.value"
      :error="errors.consent"
      :needed="!c.consented.value"
      :disabled="busy !== undefined"
      :destination="c.destination.value"
      :open="consentOpen"
      :saving="busy === 'consent'"
      @ask="consentOpen = true"
      @agree="c.agree"
      @decline="c.declineConsent"
    />
    <fieldset class="reasons" data-testid="inbox-zero-reasons">
      <legend class="sub">{{ inboxZero.settings.heading }}</legend>
      <p class="intro">{{ inboxZero.settings.intro }}</p>
      <div v-for="r in INBOX_ZERO_REASONS" :key="r" class="reason">
        <BaseSwitch
          :model-value="zero.reasons[r]"
          :ariaLabel="inboxZero.settings.reasons[r].label"
          :data-testid="`inbox-zero-reason-${r}`"
          @update:model-value="(on) => toggleReason(r, on)"
        />
        <span class="reason-text">
          <strong>{{ inboxZero.settings.reasons[r].label }}</strong>
          <span class="hint">{{ inboxZero.settings.reasons[r].hint }}</span>
        </span>
      </div>
      <p v-if="reasonsError" class="error" role="alert">{{ reasonsError }}</p>
    </fieldset>
  </section>
</template>

<style scoped>
.ai {
  display: flex;
  flex-direction: column;
  gap: var(--gap-field);
}

.group {
  display: flex;
  flex-direction: column;
  gap: var(--s-2);
}

.heading {
  margin: 0;
  font-weight: 600;
  font-size: var(--font-size-base);
}

.reasons {
  display: flex;
  flex-direction: column;
  gap: var(--s-3);
  margin: 0;
  padding: 0;
  border: none;
}

.sub {
  padding: 0;
  font-weight: 600;
  font-size: var(--font-size-base);
}

.reason {
  display: flex;
  align-items: flex-start;
  gap: var(--s-2);
}

.reason-text {
  display: flex;
  flex-direction: column;
  gap: var(--s-1);
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}

.hint {
  color: var(--mute);
  font-size: var(--font-size-xs);
  line-height: var(--line-height-xs);
}

.error {
  margin: 0;
  color: var(--danger);
  font-size: var(--font-size-sm);
}

.intro {
  margin: 0;
  color: var(--ink);
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}
</style>
