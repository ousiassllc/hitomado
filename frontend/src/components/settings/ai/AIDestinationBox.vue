<script setup lang="ts">
import { WarningTriangle } from '@iconoir/vue'
import { computed } from 'vue'
import { settings as msg } from '../../../lib/messages'
import { aiDestination } from './aiDestination'

// データの送信先（プロバイダ／Base URL）を常に表示する（NF §2）。保存済みの設定が「実際に使われる送信先」。
const props = defineProps<{
  provider: string
  baseUrl?: string
  /** 編集中（未保存）の値。保存済みと異なる場合は差を明示する。 */
  draftProvider: string
  draftBaseUrl: string
}>()

const saved = computed(() => aiDestination(props.provider, props.baseUrl))
const draft = computed(() => aiDestination(props.draftProvider, props.draftBaseUrl))
const differs = computed(
  () => draft.value.providerLabel !== saved.value.providerLabel || draft.value.url !== saved.value.url,
)
const text = (d: { providerLabel: string; url: string }) => msg.ai.destination(d.providerLabel, d.url)
</script>

<template>
  <fieldset class="dest">
    <legend class="dest-title">{{ msg.ai.destinationHeading }}</legend>
    <p class="dest-value" data-testid="ai-destination">{{ text(saved) }}</p>
    <p class="dest-note">
      {{ saved.note ?? (saved.custom ? msg.ai.destinationCustomNote : msg.ai.destinationDefaultNote) }}
    </p>
    <p v-if="differs" class="dest-pending" role="status">
      <WarningTriangle class="icon icon-sm icon-warning dest-pending-icon" aria-hidden="true" />
      <span>{{ msg.ai.destinationPending(text(draft)) }}</span>
    </p>
  </fieldset>
</template>

<style scoped>
.dest {
  padding: var(--s-3);
  border: 1px solid var(--mute);
  border-radius: var(--r-md);
  background: var(--bone);
}

.dest-title {
  padding: 0 var(--s-1);
  font-size: var(--font-size-sm);
}

.dest-value {
  margin: var(--s-1) 0;
  overflow-wrap: anywhere;
}

.dest-pending {
  display: flex;
  align-items: flex-start;
  gap: var(--s-2);
  margin: var(--s-2) 0 0;
  border: 1px solid var(--warning);
  background: var(--warning-soft);
  color: var(--ink);
  padding: var(--s-1) var(--s-2);
  border-radius: var(--r-md);
  font-size: var(--font-size-sm);
}

.dest-pending-icon {
  margin-top: var(--s-1);
}
.dest-note {
  margin: 0;
  color: var(--mute);
  font-size: var(--font-size-sm);
}
</style>
