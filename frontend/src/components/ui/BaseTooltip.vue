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
  padding: var(--space-1) var(--space-2);
  border-radius: var(--radius-sm);
  background: var(--color-text);
  color: var(--color-bg);
  font-size: var(--font-size-xs);
  z-index: 130;
}
</style>
