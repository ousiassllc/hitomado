<script setup lang="ts">
import { TooltipContent, TooltipPortal, TooltipProvider, TooltipRoot, TooltipTrigger } from 'reka-ui'

/** text 未指定ならツールチップなしでスロットをそのまま描画する */
defineProps<{ text?: string }>()
</script>

<template>
  <slot v-if="!text" />
  <TooltipProvider v-else :delay-duration="400">
    <TooltipRoot>
      <TooltipTrigger as-child>
        <slot />
      </TooltipTrigger>
      <TooltipPortal>
        <TooltipContent class="tip" :side-offset="6">{{ text }}</TooltipContent>
      </TooltipPortal>
    </TooltipRoot>
  </TooltipProvider>
</template>

<style scoped>
.tip {
  padding: var(--space-1) var(--space-2);
  border-radius: var(--radius-sm);
  background: var(--color-text);
  color: var(--color-bg);
  font-size: var(--font-size-xs);
  z-index: 130;
}
</style>
