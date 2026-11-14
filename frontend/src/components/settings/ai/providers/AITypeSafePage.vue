<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { settings as msg } from '../../../../lib/messages'
import { useUiStore } from '../../../../stores/ui'
import AIApiKey from '../AIApiKey.vue'
import AIConsentSection from '../AIConsentSection.vue'
import AIDestinationBox from '../AIDestinationBox.vue'
import { type AiBusy, useAiConnect } from '../useAiConnect'

// TypeSafe AI（Jev）の「つなぎ先」のページ（#314・#315・#376）。文章を書かないモデルなので、モデルの選択・エフォートは無く、
// 「AI で下書き」には選べない（Inbox Zero の「機能ごとに使う AI」にだけ選べる）。ここでは API キーの登録・削除と送信への同意を行う（キーは資格情報ストア）。
// Inbox Zero の理由のオン・オフは「機能ごとに使う AI」タブ（InboxZeroReasons）。
const PROVIDER = 'typesafe'
const ui = useUiStore()
onMounted(() => {
  if (ui.takeAiConnectStart()) c.begin()
})

const provider = ref(PROVIDER)
const keyForm = ref<InstanceType<typeof AIApiKey>>()
const busy = ref<AiBusy>()
const errors = ref<{ key?: string; consent?: string; baseUrl?: string }>({})
const consentOpen = ref(false)
const label = msg.ai.providerOptions[PROVIDER] ?? PROVIDER

const c = useAiConnect({ provider, busy, errors, consentOpen, keyForm, chatgptLogin: ref(undefined) })
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

.intro {
  margin: 0;
  color: var(--ink);
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}
</style>
