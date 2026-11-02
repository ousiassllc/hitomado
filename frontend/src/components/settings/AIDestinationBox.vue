<script setup lang="ts">
import { computed } from 'vue'
import { settings as msg } from '../../lib/messages'
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
      {{ saved.custom ? msg.ai.destinationCustomNote : msg.ai.destinationDefaultNote }}
    </p>
    <p v-if="differs" class="dest-pending" role="status">
      {{ msg.ai.destinationPending(text(draft)) }}
    </p>
  </fieldset>
</template>

<style scoped>
.dest {
  padding: var(--space-3);
  border: 2px solid var(--color-accent);
  border-radius: var(--radius-md);
  background: var(--color-bg-subtle);
}

.dest-title {
  padding: 0 var(--space-1);
  font-size: var(--font-size-sm);
  font-weight: 600;
}

.dest-value {
  margin: var(--space-1) 0;
  font-weight: 600;
  overflow-wrap: anywhere;
}

.dest-pending {
  margin: var(--space-2) 0 0;
  color: var(--color-warning-text);
  background: var(--color-warning-bg);
  padding: var(--space-1) var(--space-2);
  border-radius: var(--radius-sm);
  font-size: var(--font-size-sm);
}
.dest-note {
  margin: 0;
  color: var(--color-text-muted);
  font-size: var(--font-size-sm);
}
</style>
