<script setup lang="ts">
import { Check, NavArrowDown } from '@iconoir/vue'
import {
  SelectContent,
  SelectItem,
  SelectItemIndicator,
  SelectItemText,
  SelectPortal,
  SelectRoot,
  SelectTrigger,
  SelectValue,
  SelectViewport,
} from 'reka-ui'

export interface SelectOption {
  value: string
  label: string
}

defineProps<{
  modelValue?: string
  options: SelectOption[]
  placeholder?: string
  ariaLabel: string
}>()
const emit = defineEmits<{ 'update:modelValue': [value: string] }>()
</script>

<template>
  <SelectRoot :model-value="modelValue" @update:model-value="(v) => emit('update:modelValue', String(v))">
    <SelectTrigger class="sel" :aria-label="ariaLabel">
      <SelectValue :placeholder="placeholder" />
      <NavArrowDown class="icon icon-sm sel-arrow" aria-hidden="true" />
    </SelectTrigger>
    <SelectPortal>
      <SelectContent class="sel-pop" position="popper" :side-offset="4">
        <SelectViewport class="sel-viewport">
          <SelectItem v-for="o in options" :key="o.value" :value="o.value" class="sel-item">
            <SelectItemText>{{ o.label }}</SelectItemText>
            <SelectItemIndicator class="sel-check"><Check class="icon icon-sm" aria-hidden="true" /></SelectItemIndicator>
          </SelectItem>
        </SelectViewport>
      </SelectContent>
    </SelectPortal>
  </SelectRoot>
</template>

<style scoped>
.sel {
  display: inline-flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--s-2);
  min-height: var(--control-h);
  min-width: 140px;
  padding: 0 var(--s-3);
  border: 1px solid var(--border);
  border-radius: var(--r-md);
  background: var(--surface);
  color: var(--ink);
  font-size: var(--font-size-sm);
  cursor: pointer;
}

.sel:focus-visible {
  border-color: var(--focus);
  box-shadow: 0 0 0 2px var(--ring);
  outline: none;
}

.sel[data-placeholder] {
  color: var(--mute);
}

/* Reka UI の Popper は scoped 属性（data-v-*）を内側の本体ではなく外側のラッパーに付ける。本体は :global で当てる。 */
:global(.sel-pop) {
  min-width: max(var(--reka-select-trigger-width), 180px);
  max-height: 280px;
  background: var(--surface);
  color: var(--ink);
  border: 1px solid var(--border);
  border-radius: var(--r-md);
  padding: 0;
  z-index: var(--z-popup);
}

:global(.sel-pop[data-state='open']) {
  animation: ui-rise-in var(--motion-fade-in) var(--ease-out);
}

:global(.sel-pop[data-state='closed']) {
  animation: ui-sink-out var(--motion-fade-out) var(--ease-in) forwards;
}

.sel-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  min-height: var(--menu-item-h);
  padding: 0 var(--menu-pad-x);
  border-radius: var(--r-sm);
  font-size: var(--font-size-sm);
  cursor: pointer;
  outline: none;
}

/* 強調（data-highlighted）は reka がポインタの移動でも付けるので、ホバーは地だけにする（M1）。
   reka は強調した項目に実際の DOM フォーカスも当てる（ポインタ移動でも .focus() する）ので、
   キーボードの強調はブラウザの :focus-visible の判定でリングを足す。 */
.sel-item[data-highlighted] {
  background: var(--accent-soft);
}

/* リングは外側（offset 2px + 幅 2px = 4px）に描く。スクロール領域は reka の SelectViewport（overflow: hidden auto）なので、
   リングが切れないよう余白（--s-1 = 4px）はパネルではなくビューポートに持たせる。
   SelectViewport は Fragment がルートで scoped 属性（data-v-*）が付かないので、.sel-pop と同じく :global で当てる。 */
:global(.sel-viewport) {
  padding: var(--s-1);
}

.sel-item[data-highlighted]:focus-visible {
  outline: 2px solid var(--focus);
  outline-offset: 2px;
}

.sel-check {
  display: inline-flex;
  margin-left: var(--s-2);
}
</style>
