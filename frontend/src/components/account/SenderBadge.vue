<script setup lang="ts">
// アカウント色は地と枠に薄く沈めるだけ（一覧のチップと同じ）。文字は常にテーマの文字色なので、どの色でも読める。
import { accountColorVar } from '../../lib/palette'

defineProps<{
  from: { name?: string; email: string }
  color: string
}>()
</script>

<template>
  <div class="sender-badge" :style="{ '--chip-tint': accountColorVar(color) }">
    <div v-if="from.name" class="sender-name">{{ from.name }}</div>
    <div class="sender-email">{{ from.email }}</div>
  </div>
</template>

<style scoped>
/* 識別色は一覧のチップ（BaseChip の tint）と同じ沈め方：地は識別色を --surface に --tint-bg、枠は 1px の同系色を --tint-border。文字は常に --ink で、識別色を文字・帯に使わない。 */
.sender-badge {
  padding: var(--pad-card-sm);
  border: 1px solid color-mix(in srgb, var(--chip-tint) var(--tint-border), transparent);
  border-radius: var(--r-md);
  background: color-mix(in srgb, var(--chip-tint) var(--tint-bg), var(--surface));
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
