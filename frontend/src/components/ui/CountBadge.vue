<script setup lang="ts">
// 件数バッジ（受信箱の未読・エージェントの確認待ち）。--accent 地・--bone 字・--r-full・Mono・xs（docs/components/overview.md §5.3 の件数バッジ）。999 を超えたら 999+。
defineProps<{ count: number; label: string }>()
</script>

<template>
  <span v-if="count > 0" class="badge" :aria-label="label">{{ count > 999 ? '999+' : count }}</span>
</template>

<style scoped>
.badge {
  flex: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  box-sizing: border-box;
  min-width: 20px;
  min-height: 20px;
  padding: 0 var(--s-2);
  border-radius: var(--r-full);
  background: var(--accent);
  color: var(--bone);
  font-size: var(--font-size-xs);
  line-height: var(--line-height-xs);
  font-family: var(--font-mono);
  text-align: center;
}
</style>
