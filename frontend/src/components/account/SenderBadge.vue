<script setup lang="ts">
// アカウント色は左の帯だけに使う。文字は常にテーマの文字色なので、どの色でも読める。
import { accountColorVar } from '../../lib/palette'

defineProps<{
  from: { name?: string; email: string }
  color: string
}>()
</script>

<template>
  <div class="sender-badge" :style="{ borderLeftColor: accountColorVar(color) }">
    <div v-if="from.name" class="sender-name">{{ from.name }}</div>
    <div class="sender-email">{{ from.email }}</div>
  </div>
</template>

<style scoped>
.sender-badge {
  padding: var(--space-2) var(--space-3);
  border: 1px solid var(--color-border);
  border-left: var(--space-2) solid transparent;
  border-radius: var(--radius-md);
  background: var(--color-bg-subtle);
  color: var(--color-text);
  line-height: 1.35;
  overflow-wrap: anywhere;
}

.sender-name {
  font-size: var(--font-size-lg);
  font-weight: 700;
}

.sender-email {
  font-size: var(--font-size-md);
}

.sender-email:only-child {
  font-size: var(--font-size-lg);
  font-weight: 700;
}
</style>
