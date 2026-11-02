<script setup lang="ts">
import { accountColorVar, accountTextVar } from '../../lib/palette'

defineProps<{
  from: { name?: string; email: string }
  color: string
}>()
</script>

<template>
  <div class="sender-badge" :style="{ background: accountColorVar(color), color: accountTextVar(color) }">
    <div v-if="from.name" class="sender-name">{{ from.name }}</div>
    <div class="sender-email">{{ from.email }}</div>
  </div>
</template>

<style scoped>
.sender-badge {
  padding: var(--space-3) var(--space-4);
  border-radius: var(--radius-md);
  font-size: var(--font-size-sender);
  line-height: 1.25;
  font-weight: 700;
  overflow-wrap: anywhere;
}

.sender-name {
  font-size: var(--font-size-sender);
}

.sender-email {
  font-size: var(--font-size-sender);
}
</style>
