<script setup lang="ts">
import { Check } from '@iconoir/vue'
import {
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuPortal,
  DropdownMenuRoot,
  DropdownMenuTrigger,
} from 'reka-ui'

export interface MenuItem {
  key: string
  label: string
  disabled?: boolean
  danger?: boolean
  /** 選択中かどうか（ラベルが付いている等）。指定すると項目はチェック項目（menuitemcheckbox）になり、選択中は左に Check を出す */
  checked?: boolean
  /** 併記するショートカットキー（例: `r`） */
  shortcut?: string
}

defineProps<{ items: MenuItem[] }>()
const emit = defineEmits<{ select: [key: string] }>()
</script>

<template>
  <DropdownMenuRoot>
    <DropdownMenuTrigger as-child>
      <slot name="trigger" />
    </DropdownMenuTrigger>
    <DropdownMenuPortal>
      <DropdownMenuContent class="menu-pop" :side-offset="4" align="end">
        <component
          :is="item.checked === undefined ? DropdownMenuItem : DropdownMenuCheckboxItem"
          v-for="item in items"
          :key="item.key"
          v-bind="item.checked === undefined ? {} : { modelValue: item.checked }"
          class="menu-item"
          :class="{ 'menu-danger': item.danger }"
          :disabled="item.disabled"
          @select="emit('select', item.key)"
        >
          <span v-if="item.checked !== undefined" class="menu-check" aria-hidden="true"><Check v-if="item.checked" class="icon icon-sm" /></span>
          <span class="menu-label">{{ item.label }}</span>
          <kbd v-if="item.shortcut" class="menu-shortcut">{{ item.shortcut }}</kbd>
        </component>
      </DropdownMenuContent>
    </DropdownMenuPortal>
  </DropdownMenuRoot>
</template>

<style scoped>
/* Popper 本体には scoped 属性が付かない（BaseSelect.vue の .sel-pop と同じ）ので :global で当てる。 */
:global(.menu-pop) {
  min-width: 180px;
  max-height: 320px;
  overflow: auto;
  padding: var(--s-1);
  background: var(--surface);
  color: var(--ink);
  border: 1px solid var(--border);
  border-radius: var(--r-md);
  z-index: var(--z-popup);
}

:global(.menu-pop[data-state='open']) {
  animation: ui-rise-in var(--motion-fade-in) var(--ease-out);
}

:global(.menu-pop[data-state='closed']) {
  animation: ui-sink-out var(--motion-fade-out) var(--ease-in) forwards;
}

.menu-item {
  display: flex;
  align-items: center;
  min-height: var(--menu-item-h);
  padding: 0 var(--menu-pad-x);
  border-radius: var(--r-sm);
  font-size: var(--font-size-sm);
  cursor: pointer;
  outline: none;
}

.menu-check {
  display: inline-flex;
  flex: none;
  width: var(--icon-sm);
  margin-right: var(--s-2);
}

.menu-label {
  flex: 1;
}

.menu-shortcut {
  margin-left: var(--s-4);
  color: var(--mute);
  font-family: inherit;
  font-size: var(--font-size-xs);
}

/* 強調（data-highlighted）は reka がポインタの移動でも付けるので、ホバーは地だけにする（M1）。
   reka は強調した項目に実際の DOM フォーカスも当てる（ポインタ移動でも .focus() する）ので、
   キーボードの強調はブラウザの :focus-visible の判定でリングを足す。 */
.menu-item[data-highlighted] {
  background: var(--accent-soft);
}

/* --accent-soft 地の上では --mute が AA 4.5:1 に届かないので --ink にする（ラベルとは文字サイズで区別が残る）。 */
.menu-item[data-highlighted] .menu-shortcut {
  color: var(--ink);
}

/* リングは外側（offset 2px + 幅 2px = 4px）に描く。スクロール領域（.menu-pop の overflow: auto）の縁で切れないよう、パネルの padding（--s-1 = 4px）で収める。 */
.menu-item[data-highlighted]:focus-visible {
  outline: 2px solid var(--focus);
  outline-offset: 2px;
}

.menu-item[data-disabled] {
  opacity: 0.4;
  cursor: not-allowed;
}

.menu-danger {
  color: var(--danger);
}
</style>
