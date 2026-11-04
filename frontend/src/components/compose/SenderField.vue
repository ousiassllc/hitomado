<script setup lang="ts">
import { computed } from 'vue'
import { compose as msg } from '../../lib/messages'
import AccountChip from '../account/AccountChip.vue'
import BaseSelect from '../ui/BaseSelect.vue'

interface SenderAccount {
  id: string
  displayName: string
  email: string
  color: string
}

const props = defineProps<{
  /** 送信元（未選択なら undefined）。 */
  from?: { name?: string; email: string }
  /** 送信元アカウント ID（未選択なら undefined）。 */
  accountId?: string
  /** true（返信・転送）なら表示のみ。変更 UI を出さない。 */
  locked: boolean
  accounts: SenderAccount[]
}>()

const emit = defineEmits<{ select: [accountId: string] }>()

const current = computed(() => props.accounts.find((a) => a.id === props.accountId))
const options = computed(() =>
  props.accounts.map((a) => ({ value: a.id, label: `${a.displayName} <${a.email}>` })),
)
</script>

<template>
  <div class="sender-field">
    <span class="sender-label">{{ msg.sender.label }}</span>
    <template v-if="locked">
      <AccountChip v-if="current" :account="current" show-email />
      <span v-else-if="from" class="sender-email">{{ from.name ? `${from.name} <${from.email}>` : from.email }}</span>
      <span class="sender-hint">{{ msg.sender.lockedHint }}</span>
    </template>
    <template v-else>
      <BaseSelect
        class="sender-select"
        :model-value="accountId"
        :options="options"
        :placeholder="msg.sender.unselected"
        :ariaLabel="msg.sender.select"
        @update:model-value="(v: string) => emit('select', v)"
      />
      <AccountChip v-if="current" :account="current" />
    </template>
  </div>
</template>

<style scoped>
.sender-field {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-2);
  min-height: 36px;
}

.sender-label {
  flex: 0 0 56px;
  color: var(--color-text-muted);
  font-size: var(--font-size-sm);
}

.sender-email {
  font-size: var(--font-size-sm);
  font-weight: 600;
}

.sender-hint {
  color: var(--color-text-muted);
  font-size: var(--font-size-xs);
}

.sender-select {
  min-width: 260px;
}
</style>
