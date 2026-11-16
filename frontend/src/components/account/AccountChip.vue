<script setup lang="ts">
import { accountColorVar } from '../../lib/palette'
import BaseChip from '../ui/BaseChip.vue'
import BaseTooltip from '../ui/BaseTooltip.vue'

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
    :tint="accountColorVar(account.color)"
  >
    <!-- メールが並んで見えるときは足さない。ツールチップは名前の span に付け、BaseChip のルート（親の scoped class）はそのまま保つ。 -->
    <BaseTooltip :text="showEmail ? undefined : account.email">
      <span class="account-chip-name">{{ account.displayName }}</span>
    </BaseTooltip>
    <span v-if="showEmail" class="account-chip-email">{{ account.email }}</span>
  </BaseChip>
</template>

<style scoped>
.account-chip-md {
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}

.account-chip-sm {
  font-size: var(--font-size-xs);
  line-height: var(--line-height-xs);
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
