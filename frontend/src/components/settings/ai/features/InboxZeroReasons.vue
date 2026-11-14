<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { parseBindingError } from '../../../../lib/api'
import { inboxZero } from '../../../../lib/messages'
import { useInboxZeroStore } from '../../../../stores/inboxZero'
import BaseSwitch from '../../../ui/BaseSwitch.vue'
import { INBOX_ZERO_REASONS, type InboxZeroReason } from './inboxZeroReasons'

// 「機能ごとに使う AI」タブの Inbox Zero の理由のオン・オフ（#314・#377）。どの AI を選んでいても（AI を使わない理由だけのときも）ここで決める。
const zero = useInboxZeroStore()
onMounted(() => void zero.loadReasons())
const error = ref<string>()

/** 切り替えたらすぐ保存する。保存に失敗したら画面の値は変えない。 */
async function toggle(reason: InboxZeroReason, on: boolean): Promise<void> {
  error.value = undefined
  try {
    await zero.setReasons({ ...zero.reasons, [reason]: on })
  } catch (e) {
    error.value = parseBindingError(e).message || inboxZero.settings.saveFailed
  }
}
</script>

<template>
  <fieldset class="reasons" data-testid="inbox-zero-reasons">
    <legend class="sub">{{ inboxZero.settings.heading }}</legend>
    <p class="intro">{{ inboxZero.settings.intro }}</p>
    <div v-for="r in INBOX_ZERO_REASONS" :key="r" class="reason">
      <BaseSwitch
        :model-value="zero.reasons[r]"
        :ariaLabel="inboxZero.settings.reasons[r].label"
        :data-testid="`inbox-zero-reason-${r}`"
        @update:model-value="(on) => toggle(r, on)"
      />
      <span class="reason-text">
        <strong>{{ inboxZero.settings.reasons[r].label }}</strong>
        <span class="hint">{{ inboxZero.settings.reasons[r].hint }}</span>
      </span>
    </div>
    <p v-if="error" class="error" role="alert">{{ error }}</p>
  </fieldset>
</template>

<style scoped>
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
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
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
  color: var(--mute);
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}
</style>
