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
  padding: var(--pad-card-sm);
  border: 1px solid var(--border);
  border-left: var(--s-2) solid transparent;
  border-radius: var(--r-md);
  background: var(--bone);
  color: var(--ink);
  line-height: var(--line-height-sm);
  overflow-wrap: anywhere;
}

.sender-name {
  font-size: var(--font-size-lg);
}

.sender-email {
  font-size: var(--font-size-sm);
}

.sender-email:only-child {
  font-size: var(--font-size-lg);
}
</style>
