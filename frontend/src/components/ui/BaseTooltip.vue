<script setup lang="ts">
import { TooltipContent, TooltipPortal, TooltipProvider, TooltipRoot, TooltipTrigger } from 'reka-ui'
import { usePlatformStore } from '../../stores/platform'

/**
 * text 未指定ならツールチップなしでスロットをそのまま描画する。
 * タッチ UI（Android）ではホバーできず、ツールチップの中身（ショートカットキー）も使えないため出さない。
 */
defineProps<{ text?: string }>()
const platform = usePlatformStore()
</script>

<template>
  <slot v-if="!text || platform.touchUi" />
  <TooltipProvider v-else :delay-duration="400">
    <TooltipRoot>
      <TooltipTrigger as-child>
        <slot />
      </TooltipTrigger>
      <TooltipPortal>
        <TooltipContent class="tip-pop" :side-offset="6">{{ text }}</TooltipContent>
      </TooltipPortal>
    </TooltipRoot>
  </TooltipProvider>
</template>

<style scoped>
/* Popper 本体には scoped 属性が付かない（BaseSelect.vue の .sel-pop と同じ）ので :global で当てる。 */
:global(.tip-pop) {
  /* ガイドの 6×10。--s-* に 6・10 が無いので px で書く（トークン化はガイド側へ提案）。 */
  padding: 6px 10px;
  border-radius: var(--r-sm);
  background: var(--ink);
  color: var(--bone);
  font-size: var(--font-size-xs);
  z-index: var(--z-popup);
  animation: ui-fade-in var(--motion-fade-in) var(--ease-out);
}

:global(.tip-pop[data-state='closed']) {
  animation: ui-fade-out var(--motion-fade-out) var(--ease-in) forwards;
}
</style>
