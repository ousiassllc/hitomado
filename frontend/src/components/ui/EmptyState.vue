<script setup lang="ts">
/**
 * 空状態（ガイド §4・S4）。破線の枠・アイコン（`icon` スロット。Iconoir に `icon icon-lg` を付ける）・文言（既定スロット）・
 * 次の操作（`action` スロット。任意）。サイドバーのような狭い場所は `compact`（padding を `--pad-card-sm` にする。docs/components/overview.md §5.3 の逸脱表に記録済み）。
 */
defineProps<{ compact?: boolean }>()
</script>

<template>
  <div class="empty-state" :class="{ compact }">
    <span class="empty-icon" aria-hidden="true"><slot name="icon" /></span>
    <div class="empty-text"><slot /></div>
    <slot name="action" />
  </div>
</template>

<style scoped>
.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--s-3);
  padding: var(--s-12) var(--s-6);
  border: 1px dashed var(--border);
  border-radius: var(--r-lg);
  color: var(--mute);
  text-align: center;
}

.compact {
  padding: var(--pad-card-sm);
}

.empty-icon {
  display: inline-flex;
}

.empty-text {
  display: flex;
  flex-direction: column;
  gap: var(--s-1);
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}

.empty-text :deep(p) {
  margin: 0;
}
</style>
