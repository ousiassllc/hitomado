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
    <div class="dest-panel">
      <p class="dest-value" data-testid="ai-destination">{{ saved.providerLabel }}（<span class="url">{{ saved.url }}</span>）</p>
      <p class="dest-note">{{ saved.note ?? (saved.custom ? msg.ai.destinationCustomNote : msg.ai.destinationDefaultNote) }}</p>
      <Transition name="alert">
        <p v-if="differs" class="dest-pending" role="status">
          <WarningTriangle class="icon icon-sm icon-warning dest-pending-icon" aria-hidden="true" />
          <span>{{ msg.ai.destinationPending(text(draft)) }}</span>
        </p>
      </Transition>
    </div>
  </fieldset>
</template>

<style scoped>
.dest {
  margin: 0;
  padding: 0;
  border: 0;
}

.dest-title {
  margin-bottom: var(--s-2);
  padding: 0;
  font-size: var(--font-size-sm);
  font-weight: 600;
}

.dest-panel {
  padding: var(--pad-card-sm);
  border: 1px solid var(--border);
  border-radius: var(--r-md);
  background: var(--surface);
}

.dest-value {
  margin: var(--s-1) 0;
  overflow-wrap: anywhere;
}

.url {
  font-family: var(--font-mono);
}

.dest-pending {
  display: flex;
  align-items: flex-start;
  gap: var(--s-2);
  margin: var(--s-2) 0 0;
  border: 1px solid var(--warning-border);
  background: var(--warning-soft);
  color: var(--ink);
  padding: var(--s-1) var(--s-2);
  border-radius: var(--r-md);
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}

.dest-pending-icon {
  margin-top: var(--s-1);
}
.dest-note {
  margin: 0;
  color: var(--ink);
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
