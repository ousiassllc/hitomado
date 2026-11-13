<script setup lang="ts">
// 枠はダイアログと同じ中立（--surface・--border）。アカウント色は名前の薄いチップ（BaseChip の tint。一覧と同じ沈め方）にだけ使い、
// 大きな面を塗らない。メールアドレスは --ink の文字でチップの隣（収まらなければ下）に置く。名前が無いときは、アドレスをチップに入れる。
import { accountColorVar } from '../../lib/palette'
import BaseChip from '../ui/BaseChip.vue'

defineProps<{
  from: { name?: string; email: string }
  color: string
}>()
</script>

<template>
  <div class="sender-badge">
    <BaseChip class="sender-chip" :tint="accountColorVar(color)">{{ from.name || from.email }}</BaseChip>
    <span v-if="from.name" class="sender-email">{{ from.email }}</span>
  </div>
</template>

<style scoped>
.sender-badge {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--s-2);
  padding: var(--pad-card-sm);
  border: 1px solid var(--border);
  border-radius: var(--r-md);
  background: var(--surface);
  color: var(--ink);
  line-height: var(--line-height-sm);
  overflow-wrap: anywhere;
}

/* 名前（アドレスだけのときはアドレス）は省略せず折り返して全部見せる（送信元の取り違えを防ぐ）。 */
.sender-badge .sender-chip {
  padding: var(--s-1) var(--s-2);
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
  white-space: normal;
  text-overflow: clip;
}

.sender-email {
  min-width: 0;
  font-size: var(--font-size-sm);
  color: var(--ink);
}
</style>
