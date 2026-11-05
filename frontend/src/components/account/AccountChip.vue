<script setup lang="ts">
import { accountColorVar, accountTextVar } from '../../lib/palette'
import BaseChip from '../ui/BaseChip.vue'

withDefaults(
  defineProps<{
    account: { displayName: string; email: string; color: string }
    showEmail?: boolean
    size?: 'sm' | 'md'
  }>(),
  { showEmail: false, size: 'md' },
)
</script>

<template>
  <BaseChip
    class="account-chip"
    :class="`account-chip-${size}`"
    :bg="accountColorVar(account.color)"
    :fg="accountTextVar(account.color)"
    :title="account.email"
  >
    <span class="account-chip-name">{{ account.displayName }}</span>
    <span v-if="showEmail" class="account-chip-email">{{ account.email }}</span>
  </BaseChip>
</template>

<style scoped>
.account-chip {
  border-color: transparent;
}

.account-chip-md {
  font-size: var(--font-size-sm);
  line-height: 22px;
}

.account-chip-sm {
  font-size: var(--font-size-xs);
  line-height: 18px;
}

.account-chip-name {
  overflow: hidden;
  text-overflow: ellipsis;
}

.account-chip-email {
  overflow: hidden;
  text-overflow: ellipsis;
  opacity: 0.9;
}
</style>
